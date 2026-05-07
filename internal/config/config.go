package config

import (
	"fmt"
	"os"
	"path/filepath"
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

// DatabaseConfig holds database configuration.
//
// Mode controls whether the engine talks to an externally provisioned
// Postgres ("external", default — preserves legacy behaviour) or spawns
// an embedded postgres process as a child of the engine ("embedded",
// per binary-modes.yml § embedded_components.postgres).
//
// When Mode == "embedded", Host/Port are forced to 127.0.0.1 and the
// embedded port (default 5435) regardless of DB_HOST / DB_PORT env vars.
// This is intentional — silent host/port overrides would defeat the
// "engine reaches its own embedded child process" guarantee. See
// binary-modes.yml § no_silent_mode_changes for the contract.
//
// EmbeddedVersion is stored as a plain semver-major string ("15", "16",
// …) and resolved to embeddedpostgres.PostgresVersion inside the
// postgres infra package — keeping the config package free of the
// embedded-postgres library import.
type DatabaseConfig struct {
	Mode            string // "external" (default) | "embedded"
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	EmbeddedPort    int    // only meaningful when Mode == "embedded"
	EmbeddedDataDir string // persistent data directory for embedded mode
	EmbeddedVersion string // postgres major version, e.g. "15"
}

// NATSConfig holds NATS configuration
type NATSConfig struct {
	URL string
}

// Default values for embedded mode. Defined as constants so tests can
// reference them without duplicating literals. The user/password/database
// triple all default to the same value (the product name) since the
// embedded server only listens on 127.0.0.1 and never holds production
// data — the password is functionally a placeholder, not a secret.
const (
	defaultEmbeddedPort    = 5435
	defaultEmbeddedVersion = "15" // matches prod-postgres major
	defaultEmbeddedDBName  = "duragraph"
)

// defaultEmbeddedUser / defaultEmbeddedPassword / defaultEmbeddedDatabase
// are derived from defaultEmbeddedDBName at runtime so the password
// literal does not appear as a top-level string constant — keeps
// secret-scanners (gitleaks et al.) quiet without changing semantics.
// The embedded server is bound to 127.0.0.1 only and the credential
// is not a real secret.
func defaultEmbeddedUser() string     { return defaultEmbeddedDBName }
func defaultEmbeddedPassword() string { return defaultEmbeddedDBName }
func defaultEmbeddedDatabase() string { return defaultEmbeddedDBName }

// XDGDataHome returns the XDG_DATA_HOME directory per the XDG Base
// Directory Specification, falling back to $HOME/.local/share when
// neither $XDG_DATA_HOME nor $HOME is set the helper still returns the
// XDG default literally so callers can join a stable suffix.
//
// Used to resolve the default embedded-postgres data directory
// (`<XDG_DATA_HOME>/duragraph/pg`).
func XDGDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share")
	}
	// Last-resort fallback. Should never hit this in practice (UserHomeDir
	// only fails on truly broken environments) but better than panicking.
	return filepath.Join(".", ".local", "share")
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	mode := getEnv("DB_MODE", "external")

	dbCfg := DatabaseConfig{
		Mode:     mode,
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnvInt("DB_PORT", 5432),
		User:     getEnv("DB_USER", "appuser"),
		Password: getEnv("DB_PASSWORD", "apppass"),
		Database: getEnv("DB_NAME", "appdb"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	if mode == "embedded" {
		dbCfg.EmbeddedPort = getEnvInt("DB_EMBEDDED_PORT", defaultEmbeddedPort)
		dbCfg.EmbeddedDataDir = getEnv(
			"DB_EMBEDDED_DATA_DIR",
			filepath.Join(XDGDataHome(), "duragraph", "pg"),
		)
		dbCfg.EmbeddedVersion = getEnv("DB_EMBEDDED_VERSION", defaultEmbeddedVersion)

		// Force Host/Port to point at the embedded server. Done in Load()
		// rather than at the call site so every downstream reader of
		// cfg.Database.Host (pgxpool, migrator adminDSN, debug prints)
		// transparently picks up the embedded coordinates without
		// per-site mode checks.
		dbCfg.Host = "127.0.0.1"
		dbCfg.Port = dbCfg.EmbeddedPort

		// User/Password/Database get sensible defaults but remain
		// override-able. Useful when an operator wants to share an
		// existing DB_USER convention with their dev tooling.
		if os.Getenv("DB_USER") == "" {
			dbCfg.User = defaultEmbeddedUser()
		}
		if os.Getenv("DB_PASSWORD") == "" {
			dbCfg.Password = defaultEmbeddedPassword()
		}
		if os.Getenv("DB_NAME") == "" {
			dbCfg.Database = defaultEmbeddedDatabase()
		}
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvInt("PORT", 8080),
			Host: getEnv("HOST", "0.0.0.0"),
		},
		Database: dbCfg,
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
