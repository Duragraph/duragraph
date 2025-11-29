package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// SimpleLimiter is a simple in-memory rate limiter
type SimpleLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewSimpleLimiter creates a new simple rate limiter
func NewSimpleLimiter(r rate.Limit, b int) *SimpleLimiter {
	return &SimpleLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
	}
}

// GetLimiter returns a limiter for a key
func (l *SimpleLimiter) GetLimiter(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	limiter, exists := l.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(l.rate, l.burst)
		l.limiters[key] = limiter
	}

	return limiter
}

// CleanupRoutine periodically removes inactive limiters
func (l *SimpleLimiter) CleanupRoutine(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.mu.Lock()
			// Simple cleanup: remove all limiters (they'll be recreated if needed)
			l.limiters = make(map[string]*rate.Limiter)
			l.mu.Unlock()
		}
	}
}

// SimpleRateLimit creates a simple rate limiting middleware
func SimpleRateLimit(requestsPerSecond float64, burst int) echo.MiddlewareFunc {
	limiter := NewSimpleLimiter(rate.Limit(requestsPerSecond), burst)

	// Start cleanup routine
	ctx := context.Background()
	go limiter.CleanupRoutine(ctx, 10*time.Minute)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip for health/metrics
			if c.Path() == "/health" || c.Path() == "/metrics" {
				return next(c)
			}

			// Get key (IP or user ID)
			key := c.RealIP()
			if userID := c.Get("user_id"); userID != nil {
				key = fmt.Sprintf("user:%v", userID)
			}

			// Get limiter and check
			l := limiter.GetLimiter(key)
			if !l.Allow() {
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"error":   "rate_limit_exceeded",
					"message": "Too many requests. Please slow down.",
				})
			}

			return next(c)
		}
	}
}

// RedisRateLimiter uses Redis for distributed rate limiting
type RedisRateLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

// NewRedisRateLimiter creates a Redis-based rate limiter
func NewRedisRateLimiter(client *redis.Client, limit int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		client: client,
		limit:  limit,
		window: window,
	}
}

// Allow checks if a request is allowed
func (r *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().Unix()
	windowStart := now - int64(r.window.Seconds())

	pipe := r.client.Pipeline()

	// Remove old entries
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))

	// Count current requests
	countCmd := pipe.ZCard(ctx, key)

	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now),
		Member: fmt.Sprintf("%d", now),
	})

	// Set expiration
	pipe.Expire(ctx, key, r.window)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	count := countCmd.Val()
	return int(count) < r.limit, nil
}

// RedisRateLimit creates a Redis-based rate limiting middleware
func RedisRateLimit(redisClient *redis.Client, limit int, window time.Duration) echo.MiddlewareFunc {
	limiter := NewRedisRateLimiter(redisClient, limit, window)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip for health/metrics
			if c.Path() == "/health" || c.Path() == "/metrics" {
				return next(c)
			}

			// Get key
			key := c.RealIP()
			if userID := c.Get("user_id"); userID != nil {
				key = fmt.Sprintf("ratelimit:user:%v", userID)
			} else {
				key = fmt.Sprintf("ratelimit:ip:%s", key)
			}

			// Check limit
			allowed, err := limiter.Allow(c.Request().Context(), key)
			if err != nil {
				// On error, allow the request
				return next(c)
			}

			if !allowed {
				c.Response().Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
				c.Response().Header().Set("X-RateLimit-Window", window.String())

				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"error":   "rate_limit_exceeded",
					"message": fmt.Sprintf("Rate limit exceeded. Maximum %d requests per %s.", limit, window),
					"limit":   limit,
					"window":  window.String(),
				})
			}

			return next(c)
		}
	}
}

// TieredRateLimitSimple creates tiered rate limits
func TieredRateLimitSimple(redisClient *redis.Client) echo.MiddlewareFunc {
	freeLimiter := NewRedisRateLimiter(redisClient, 10, 1*time.Minute)
	proLimiter := NewRedisRateLimiter(redisClient, 100, 1*time.Minute)
	enterpriseLimiter := NewRedisRateLimiter(redisClient, 1000, 1*time.Minute)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip for health/metrics
			if c.Path() == "/health" || c.Path() == "/metrics" {
				return next(c)
			}

			// Determine tier
			tier := "free"
			limit := 10
			var limiter *RedisRateLimiter = freeLimiter

			if roles, ok := c.Get("roles").([]string); ok {
				for _, role := range roles {
					if role == "enterprise" {
						tier = "enterprise"
						limit = 1000
						limiter = enterpriseLimiter
						break
					} else if role == "pro" {
						tier = "pro"
						limit = 100
						limiter = proLimiter
					}
				}
			}

			// Get key
			key := c.RealIP()
			if userID := c.Get("user_id"); userID != nil {
				key = fmt.Sprintf("ratelimit:%s:user:%v", tier, userID)
			} else {
				key = fmt.Sprintf("ratelimit:%s:ip:%s", tier, key)
			}

			// Check limit
			allowed, err := limiter.Allow(c.Request().Context(), key)
			if err != nil {
				return next(c)
			}

			// Set headers
			c.Response().Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			c.Response().Header().Set("X-RateLimit-Tier", tier)

			if !allowed {
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"error":   "rate_limit_exceeded",
					"message": fmt.Sprintf("Rate limit exceeded for %s tier.", tier),
					"tier":    tier,
					"limit":   limit,
				})
			}

			return next(c)
		}
	}
}
