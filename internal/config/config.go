package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds application configuration
type Config struct {
	Server       ServerConfig
	Database     DatabaseConfig
	ReadDatabase *DatabaseConfig
	NATS         NATSConfig
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port int
	Host string
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

// NATSConfig holds NATS configuration
type NATSConfig struct {
	URL string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvInt("PORT", 8080),
			Host: getEnv("HOST", "0.0.0.0"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "appuser"),
			Password: getEnv("DB_PASSWORD", "apppass"),
			Database: getEnv("DB_NAME", "appdb"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		NATS: NATSConfig{
			URL: getEnv("NATS_URL", "nats://localhost:4222"),
		},
	}

	if readHost := os.Getenv("DB_READ_HOST"); readHost != "" {
		cfg.ReadDatabase = &DatabaseConfig{
			Host:     readHost,
			Port:     getEnvInt("DB_READ_PORT", cfg.Database.Port),
			User:     getEnv("DB_READ_USER", cfg.Database.User),
			Password: getEnv("DB_READ_PASSWORD", cfg.Database.Password),
			Database: getEnv("DB_READ_NAME", cfg.Database.Database),
			SSLMode:  getEnv("DB_READ_SSLMODE", cfg.Database.SSLMode),
		}
	}

	return cfg, nil
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an integer environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// ServerAddr returns the server address
func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
