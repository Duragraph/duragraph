package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	os.Unsetenv("PORT")
	os.Unsetenv("HOST")
	os.Unsetenv("DB_MODE")
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
	if cfg.Database.Mode != "external" {
		t.Errorf("default mode: got %q, want external", cfg.Database.Mode)
	}
}

// TestLoad_EmbeddedDefaults verifies that DB_MODE=embedded picks up the
// spec-defined defaults: 127.0.0.1, port 5435, version 15, and the
// duragraph/duragraph/duragraph credential triple.
func TestLoad_EmbeddedDefaults(t *testing.T) {
	t.Setenv("DB_MODE", "embedded")
	// Explicitly unset everything else so we observe the embedded
	// defaults rather than carry-over from the surrounding process env.
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USER")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")
	os.Unsetenv("DB_EMBEDDED_PORT")
	os.Unsetenv("DB_EMBEDDED_DATA_DIR")
	os.Unsetenv("DB_EMBEDDED_VERSION")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.Database.Mode; got != "embedded" {
		t.Errorf("Mode: got %q, want embedded", got)
	}
	if got := cfg.Database.Host; got != "127.0.0.1" {
		t.Errorf("Host: got %q, want 127.0.0.1 (forced in embedded mode)", got)
	}
	if got := cfg.Database.Port; got != 5435 {
		t.Errorf("Port: got %d, want 5435 (default embedded port)", got)
	}
	if got := cfg.Database.EmbeddedPort; got != 5435 {
		t.Errorf("EmbeddedPort: got %d, want 5435", got)
	}
	if got := cfg.Database.User; got != "duragraph" {
		t.Errorf("User: got %q, want duragraph", got)
	}
	if got := cfg.Database.Password; got != "duragraph" {
		t.Errorf("Password: got %q, want duragraph", got)
	}
	if got := cfg.Database.Database; got != "duragraph" {
		t.Errorf("Database: got %q, want duragraph", got)
	}
	if got := cfg.Database.EmbeddedVersion; got != "15" {
		t.Errorf("EmbeddedVersion: got %q, want 15", got)
	}
	if cfg.Database.EmbeddedDataDir == "" {
		t.Error("EmbeddedDataDir should default to <XDGDataHome>/duragraph/pg")
	}
	wantSuffix := filepath.Join("duragraph", "pg")
	if !filepath.IsAbs(cfg.Database.EmbeddedDataDir) {
		t.Errorf("EmbeddedDataDir should be absolute: got %q", cfg.Database.EmbeddedDataDir)
	}
	if filepath.Base(filepath.Dir(cfg.Database.EmbeddedDataDir))+
		string(filepath.Separator)+filepath.Base(cfg.Database.EmbeddedDataDir) != wantSuffix {
		t.Errorf("EmbeddedDataDir suffix: got %q, want suffix %q", cfg.Database.EmbeddedDataDir, wantSuffix)
	}
}

// TestLoad_EmbeddedForcesHostAndPort enforces the silent-mode-change
// guardrail in binary-modes.yml: when DB_MODE=embedded, an operator's
// stale DB_HOST / DB_PORT must NOT leak through. The engine has to
// reach its own embedded child process — anything else defeats the
// whole mode.
func TestLoad_EmbeddedForcesHostAndPort(t *testing.T) {
	t.Setenv("DB_MODE", "embedded")
	t.Setenv("DB_HOST", "should-be-ignored.example.com")
	t.Setenv("DB_PORT", "9999")
	t.Setenv("DB_EMBEDDED_PORT", "5436")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.Database.Host; got != "127.0.0.1" {
		t.Errorf("Host should be forced to 127.0.0.1 in embedded mode, got %q", got)
	}
	if got := cfg.Database.Port; got != 5436 {
		t.Errorf("Port should be forced to EmbeddedPort (5436), got %d", got)
	}
	if got := cfg.Database.EmbeddedPort; got != 5436 {
		t.Errorf("EmbeddedPort: got %d, want 5436", got)
	}
}

// TestLoad_EmbeddedHonoursCredentialOverrides verifies that operators
// can still override DB_USER / DB_PASSWORD / DB_NAME in embedded mode —
// only Host/Port are forced. This keeps embedded mode usable when
// shared dev tooling has its own user/database conventions.
func TestLoad_EmbeddedHonoursCredentialOverrides(t *testing.T) {
	t.Setenv("DB_MODE", "embedded")
	t.Setenv("DB_USER", "custom_user")
	t.Setenv("DB_PASSWORD", "custom_pass")
	t.Setenv("DB_NAME", "custom_db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.Database.User; got != "custom_user" {
		t.Errorf("User override should win: got %q", got)
	}
	if got := cfg.Database.Password; got != "custom_pass" {
		t.Errorf("Password override should win: got %q", got)
	}
	if got := cfg.Database.Database; got != "custom_db" {
		t.Errorf("Database override should win: got %q", got)
	}
}

// TestLoad_EmbeddedDataDirOverride verifies DB_EMBEDDED_DATA_DIR wins
// over the XDG-derived default.
func TestLoad_EmbeddedDataDirOverride(t *testing.T) {
	t.Setenv("DB_MODE", "embedded")
	t.Setenv("DB_EMBEDDED_DATA_DIR", "/var/lib/duragraph-pg")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.Database.EmbeddedDataDir; got != "/var/lib/duragraph-pg" {
		t.Errorf("EmbeddedDataDir: got %q, want /var/lib/duragraph-pg", got)
	}
}

// TestXDGDataHome covers the three branches: env var set, env var
// unset (fall back to $HOME/.local/share), and HOME unset (last-resort
// fallback). The third branch is only exercised on the rare CI runner
// without HOME set.
func TestXDGDataHome(t *testing.T) {
	t.Run("XDG_DATA_HOME set", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/custom/xdg/data")
		if got := XDGDataHome(); got != "/custom/xdg/data" {
			t.Errorf("got %q, want /custom/xdg/data", got)
		}
	})

	t.Run("XDG_DATA_HOME unset, HOME set", func(t *testing.T) {
		os.Unsetenv("XDG_DATA_HOME")
		// UserHomeDir reads HOME; setting HOME explicitly makes the test
		// hermetic.
		t.Setenv("HOME", "/home/testuser")
		want := filepath.Join("/home/testuser", ".local", "share")
		if got := XDGDataHome(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
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
