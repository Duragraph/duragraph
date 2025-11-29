package middleware

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// JWTClaims represents the claims in a JWT token
type JWTClaims struct {
	UserID   string   `json:"user_id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret    string
	RequireAuth  bool
	AllowedRoles []string
	SkipPaths    []string
	APIKeyHeader string
	ValidAPIKeys map[string]bool
}

// JWT creates a JWT authentication middleware
func JWT(config AuthConfig) echo.MiddlewareFunc {
	if config.APIKeyHeader == "" {
		config.APIKeyHeader = "X-API-Key"
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip authentication for certain paths
			path := c.Path()
			for _, skipPath := range config.SkipPaths {
				if strings.HasPrefix(path, skipPath) {
					return next(c)
				}
			}

			// Check for API key first
			apiKey := c.Request().Header.Get(config.APIKeyHeader)
			if apiKey != "" {
				if config.ValidAPIKeys[apiKey] {
					// Set a simple user context for API key auth
					c.Set("user_id", "api_key_user")
					c.Set("auth_type", "api_key")
					return next(c)
				}
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid API key")
			}

			// Check for JWT token
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				if config.RequireAuth {
					return echo.NewHTTPError(http.StatusUnauthorized, "Missing authorization header")
				}
				return next(c)
			}

			// Extract token from "Bearer <token>"
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid authorization header format")
			}

			tokenString := parts[1]

			// Parse and validate token
			token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
				// Validate signing method
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, echo.NewHTTPError(http.StatusUnauthorized, "Invalid signing method")
				}
				return []byte(config.JWTSecret), nil
			})

			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid token: "+err.Error())
			}

			if !token.Valid {
				return echo.NewHTTPError(http.StatusUnauthorized, "Token is not valid")
			}

			// Extract claims
			claims, ok := token.Claims.(*JWTClaims)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid token claims")
			}

			// Check roles if specified
			if len(config.AllowedRoles) > 0 {
				hasRole := false
				for _, userRole := range claims.Roles {
					for _, allowedRole := range config.AllowedRoles {
						if userRole == allowedRole {
							hasRole = true
							break
						}
					}
					if hasRole {
						break
					}
				}

				if !hasRole {
					return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
				}
			}

			// Set user context
			c.Set("user_id", claims.UserID)
			c.Set("username", claims.Username)
			c.Set("email", claims.Email)
			c.Set("roles", claims.Roles)
			c.Set("auth_type", "jwt")

			return next(c)
		}
	}
}

// RequireAuth is a convenience middleware that requires authentication
func RequireAuth(jwtSecret string) echo.MiddlewareFunc {
	return JWT(AuthConfig{
		JWTSecret:   jwtSecret,
		RequireAuth: true,
		SkipPaths:   []string{"/health", "/metrics"},
	})
}

// OptionalAuth is a convenience middleware that allows optional authentication
func OptionalAuth(jwtSecret string) echo.MiddlewareFunc {
	return JWT(AuthConfig{
		JWTSecret:   jwtSecret,
		RequireAuth: false,
		SkipPaths:   []string{"/health", "/metrics"},
	})
}

// APIKeyAuth creates an API key only authentication middleware
func APIKeyAuth(validAPIKeys []string) echo.MiddlewareFunc {
	keyMap := make(map[string]bool)
	for _, key := range validAPIKeys {
		keyMap[key] = true
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip for health and metrics
			if strings.HasPrefix(c.Path(), "/health") || strings.HasPrefix(c.Path(), "/metrics") {
				return next(c)
			}

			apiKey := c.Request().Header.Get("X-API-Key")
			if apiKey == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Missing API key")
			}

			if !keyMap[apiKey] {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid API key")
			}

			c.Set("user_id", "api_key_user")
			c.Set("auth_type", "api_key")

			return next(c)
		}
	}
}
