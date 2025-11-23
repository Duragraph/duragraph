package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache wraps Redis client for caching
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(addr, password string, db int) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisCache{
		client: client,
	}, nil
}

// Set stores a value with expiration
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, expiration).Err()
}

// Get retrieves a value
func (r *RedisCache) Get(ctx context.Context, key string) (interface{}, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}

	return value, nil
}

// GetString retrieves a string value
func (r *RedisCache) GetString(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

// Delete removes a key
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// Exists checks if a key exists
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}

// Incr increments a counter
func (r *RedisCache) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

// Expire sets expiration on a key
func (r *RedisCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.client.Expire(ctx, key, expiration).Err()
}

// Close closes the Redis connection
func (r *RedisCache) Close() error {
	return r.client.Close()
}

// Client returns the underlying Redis client
func (r *RedisCache) Client() *redis.Client {
	return r.client
}

// RedisStateStore implements OAuth StateStore using Redis
type RedisStateStore struct {
	cache *RedisCache
}

// NewRedisStateStore creates a new Redis state store
func NewRedisStateStore(cache *RedisCache) *RedisStateStore {
	return &RedisStateStore{
		cache: cache,
	}
}

// Set stores OAuth state
func (s *RedisStateStore) Set(ctx context.Context, state string, data interface{}, expiration time.Duration) error {
	key := "oauth:state:" + state
	return s.cache.Set(ctx, key, data, expiration)
}

// Get retrieves OAuth state
func (s *RedisStateStore) Get(ctx context.Context, state string) (interface{}, error) {
	key := "oauth:state:" + state
	return s.cache.Get(ctx, key)
}

// Delete removes OAuth state
func (s *RedisStateStore) Delete(ctx context.Context, state string) error {
	key := "oauth:state:" + state
	return s.cache.Delete(ctx, key)
}
