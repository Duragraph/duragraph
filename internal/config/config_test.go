package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	os.Unsetenv("PORT")
	os.Unsetenv("HOST")
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USER")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")
	os.Unsetenv("DB_SSLMODE")
	os.Unsetenv("NATS_URL")
	os.Unsetenv("DB_READ_HOST")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("default port: got %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("default host: got %q", cfg.Server.Host)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("default db host: got %q", cfg.Database.Host)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("default db port: got %d", cfg.Database.Port)
	}
	if cfg.Database.User != "appuser" {
		t.Errorf("default db user: got %q", cfg.Database.User)
	}
	if cfg.Database.Password != "apppass" {
		t.Errorf("default db password: got %q", cfg.Database.Password)
	}
	if cfg.Database.Database != "appdb" {
		t.Errorf("default db name: got %q", cfg.Database.Database)
	}
	if cfg.Database.SSLMode != "disable" {
		t.Errorf("default sslmode: got %q", cfg.Database.SSLMode)
	}
	if cfg.NATS.URL != "nats://localhost:4222" {
		t.Errorf("default nats url: got %q", cfg.NATS.URL)
	}
	if cfg.ReadDatabase != nil {
		t.Error("read database should be nil when DB_READ_HOST not set")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "myuser")
	t.Setenv("DB_PASSWORD", "mypass")
	t.Setenv("DB_NAME", "mydb")
	t.Setenv("DB_SSLMODE", "require")
	t.Setenv("NATS_URL", "nats://nats.example.com:4222")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("port: got %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("host: got %q", cfg.Server.Host)
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("db host: got %q", cfg.Database.Host)
	}
	if cfg.Database.Port != 5433 {
		t.Errorf("db port: got %d", cfg.Database.Port)
	}
	if cfg.Database.SSLMode != "require" {
		t.Errorf("sslmode: got %q", cfg.Database.SSLMode)
	}
}

func TestLoad_ReadReplica(t *testing.T) {
	t.Setenv("DB_HOST", "primary.db")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "writer")
	t.Setenv("DB_PASSWORD", "writerpass")
	t.Setenv("DB_NAME", "maindb")
	t.Setenv("DB_SSLMODE", "require")
	t.Setenv("DB_READ_HOST", "replica.db")
	t.Setenv("DB_READ_PORT", "5433")
	t.Setenv("DB_READ_USER", "reader")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ReadDatabase == nil {
		t.Fatal("read database should not be nil")
	}
	if cfg.ReadDatabase.Host != "replica.db" {
		t.Errorf("read host: got %q", cfg.ReadDatabase.Host)
	}
	if cfg.ReadDatabase.Port != 5433 {
		t.Errorf("read port: got %d", cfg.ReadDatabase.Port)
	}
	if cfg.ReadDatabase.User != "reader" {
		t.Errorf("read user: got %q", cfg.ReadDatabase.User)
	}
	if cfg.ReadDatabase.Password != "writerpass" {
		t.Errorf("read password should inherit from primary: got %q", cfg.ReadDatabase.Password)
	}
	if cfg.ReadDatabase.Database != "maindb" {
		t.Errorf("read database should inherit from primary: got %q", cfg.ReadDatabase.Database)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	t.Setenv("PORT", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("invalid PORT should fall back to default: got %d", cfg.Server.Port)
	}
}

func TestServerAddr(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
	}

	addr := cfg.ServerAddr()
	if addr != "0.0.0.0:8080" {
		t.Errorf("expected 0.0.0.0:8080, got %q", addr)
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("TEST_VAR", "custom")
	if v := getEnv("TEST_VAR", "default"); v != "custom" {
		t.Errorf("expected custom, got %q", v)
	}

	os.Unsetenv("TEST_VAR_MISSING")
	if v := getEnv("TEST_VAR_MISSING", "fallback"); v != "fallback" {
		t.Errorf("expected fallback, got %q", v)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if v := getEnvInt("TEST_INT", 0); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}

	t.Setenv("TEST_INT_BAD", "abc")
	if v := getEnvInt("TEST_INT_BAD", 99); v != 99 {
		t.Errorf("expected 99 for invalid int, got %d", v)
	}

	os.Unsetenv("TEST_INT_MISSING")
	if v := getEnvInt("TEST_INT_MISSING", 7); v != 7 {
		t.Errorf("expected 7 for missing var, got %d", v)
	}
}
