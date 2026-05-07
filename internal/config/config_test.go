package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	os.Unsetenv("NATS_MODE")
	os.Unsetenv("DB_READ_HOST")
	os.Unsetenv("MIGRATOR_PLATFORM_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.NATS.Mode != "external" {
		t.Errorf("default NATS mode: got %q, want external", cfg.NATS.Mode)
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
	os.Unsetenv("DB_SSLMODE")
	os.Unsetenv("DB_EMBEDDED_PORT")
	os.Unsetenv("DB_EMBEDDED_DATA_DIR")
	os.Unsetenv("DB_EMBEDDED_VERSION")
	os.Unsetenv("DB_EMBEDDED_START_TIMEOUT")

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
	// Embedded mode must force SSLMode=disable regardless of the
	// process default, since the embedded postgres has no SSL
	// configured.
	if got := cfg.Database.SSLMode; got != "disable" {
		t.Errorf("SSLMode: got %q, want disable (forced in embedded mode)", got)
	}
	if got := cfg.Database.EmbeddedStartTimeout; got != 60*time.Second {
		t.Errorf("EmbeddedStartTimeout: got %v, want 60s", got)
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

// TestLoad_EmbeddedForcesSSLModeDisable verifies that an operator's
// stale DB_SSLMODE=require (left over from external setup) does NOT
// leak through to the embedded-mode connection. The embedded postgres
// has no SSL configured, so anything but "disable" causes a confusing
// "server does not support SSL" error at first connect.
func TestLoad_EmbeddedForcesSSLModeDisable(t *testing.T) {
	t.Setenv("DB_MODE", "embedded")
	t.Setenv("DB_SSLMODE", "require")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.Database.SSLMode; got != "disable" {
		t.Errorf("SSLMode should be forced to 'disable' in embedded mode, got %q", got)
	}
}

// TestLoad_EmbeddedStartTimeoutOverride verifies that
// DB_EMBEDDED_START_TIMEOUT (Go duration syntax) plumbs through to
// EmbeddedStartTimeout.
func TestLoad_EmbeddedStartTimeoutOverride(t *testing.T) {
	t.Setenv("DB_MODE", "embedded")
	t.Setenv("DB_EMBEDDED_START_TIMEOUT", "90s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.Database.EmbeddedStartTimeout; got != 90*time.Second {
		t.Errorf("EmbeddedStartTimeout: got %v, want 90s", got)
	}
}

// TestLoad_RejectsBadMode covers Fix 5: DB_MODE typos must fail loud
// at startup rather than silently behaving like "external".
func TestLoad_RejectsBadMode(t *testing.T) {
	t.Setenv("DB_MODE", "embedd")

	cfg, err := Load()
	if err == nil {
		t.Fatalf("expected error for DB_MODE=embedd, got cfg=%+v", cfg)
	}
	if !strings.Contains(err.Error(), "DB_MODE") {
		t.Errorf("error should mention DB_MODE, got: %v", err)
	}
}

// TestLoad_RejectsBadEmbeddedPort covers Fix 4: out-of-range
// EmbeddedPort values must be rejected before downstream code casts to
// uint32 (which would silently wrap negatives and >65535 values).
func TestLoad_RejectsBadEmbeddedPort(t *testing.T) {
	cases := []struct {
		name string
		port string
	}{
		{"zero", "0"},
		{"negative", "-1"},
		{"too-large", "70000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DB_MODE", "embedded")
			t.Setenv("DB_EMBEDDED_PORT", tc.port)

			cfg, err := Load()
			if err == nil {
				t.Fatalf("expected error for DB_EMBEDDED_PORT=%s, got cfg=%+v", tc.port, cfg)
			}
			if !strings.Contains(err.Error(), "DB_EMBEDDED_PORT") {
				t.Errorf("error should mention DB_EMBEDDED_PORT, got: %v", err)
			}
		})
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

// TestLoad_EmbeddedNATSDefaults verifies that NATS_MODE=embedded picks
// up the spec-defined defaults: 127.0.0.1, port 4222, monitoring off,
// and the XDG-derived data directory.
func TestLoad_EmbeddedNATSDefaults(t *testing.T) {
	t.Setenv("NATS_MODE", "embedded")
	// Unset everything else so we observe the embedded defaults rather
	// than carry-over from the surrounding process env.
	os.Unsetenv("NATS_URL")
	os.Unsetenv("NATS_EMBEDDED_PORT")
	os.Unsetenv("NATS_EMBEDDED_DATA_DIR")
	os.Unsetenv("NATS_EMBEDDED_MONITOR_PORT")
	os.Unsetenv("NATS_EMBEDDED_START_TIMEOUT")
	os.Unsetenv("MIGRATOR_PLATFORM_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.NATS.Mode; got != "embedded" {
		t.Errorf("Mode: got %q, want embedded", got)
	}
	if got := cfg.NATS.EmbeddedPort; got != 4222 {
		t.Errorf("EmbeddedPort: got %d, want 4222", got)
	}
	if got := cfg.NATS.URL; got != "nats://127.0.0.1:4222" {
		t.Errorf("URL: got %q, want nats://127.0.0.1:4222 (forced in embedded mode)", got)
	}
	if got := cfg.NATS.EmbeddedMonitorPort; got != 0 {
		t.Errorf("EmbeddedMonitorPort: got %d, want 0 (disabled by default)", got)
	}
	if cfg.NATS.EmbeddedDataDir == "" {
		t.Error("EmbeddedDataDir should default to <XDGDataHome>/duragraph/nats")
	}
	if !filepath.IsAbs(cfg.NATS.EmbeddedDataDir) {
		t.Errorf("EmbeddedDataDir should be absolute: got %q", cfg.NATS.EmbeddedDataDir)
	}
	wantSuffix := filepath.Join("duragraph", "nats")
	gotSuffix := filepath.Base(filepath.Dir(cfg.NATS.EmbeddedDataDir)) +
		string(filepath.Separator) + filepath.Base(cfg.NATS.EmbeddedDataDir)
	if gotSuffix != wantSuffix {
		t.Errorf("EmbeddedDataDir suffix: got suffix %q, want suffix %q (full path %q)",
			gotSuffix, wantSuffix, cfg.NATS.EmbeddedDataDir)
	}
	if got := cfg.NATS.EmbeddedStartTimeout; got != 10*time.Second {
		t.Errorf("EmbeddedStartTimeout: got %v, want 10s", got)
	}
}

// TestLoad_EmbeddedNATSForcesURL enforces the silent-mode-change
// guardrail in binary-modes.yml: when NATS_MODE=embedded, an operator's
// stale NATS_URL must NOT leak through. The engine has to dial its own
// in-process server.
func TestLoad_EmbeddedNATSForcesURL(t *testing.T) {
	t.Setenv("NATS_MODE", "embedded")
	t.Setenv("NATS_URL", "nats://should-be-ignored.example.com:4222")
	t.Setenv("NATS_EMBEDDED_PORT", "4223")
	os.Unsetenv("MIGRATOR_PLATFORM_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.NATS.URL; got != "nats://127.0.0.1:4223" {
		t.Errorf("URL should be forced to nats://127.0.0.1:4223 in embedded mode, got %q", got)
	}
	if got := cfg.NATS.EmbeddedPort; got != 4223 {
		t.Errorf("EmbeddedPort: got %d, want 4223", got)
	}
}

// TestLoad_EmbeddedNATSDataDirOverride verifies NATS_EMBEDDED_DATA_DIR
// wins over the XDG-derived default.
func TestLoad_EmbeddedNATSDataDirOverride(t *testing.T) {
	t.Setenv("NATS_MODE", "embedded")
	t.Setenv("NATS_EMBEDDED_DATA_DIR", "/var/lib/duragraph-nats")
	os.Unsetenv("MIGRATOR_PLATFORM_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.NATS.EmbeddedDataDir; got != "/var/lib/duragraph-nats" {
		t.Errorf("EmbeddedDataDir: got %q, want /var/lib/duragraph-nats", got)
	}
}

// TestLoad_EmbeddedNATSMonitorPort verifies that opting into the
// HTTP /varz monitor port plumbs through correctly.
func TestLoad_EmbeddedNATSMonitorPort(t *testing.T) {
	t.Setenv("NATS_MODE", "embedded")
	t.Setenv("NATS_EMBEDDED_MONITOR_PORT", "8222")
	os.Unsetenv("MIGRATOR_PLATFORM_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.NATS.EmbeddedMonitorPort; got != 8222 {
		t.Errorf("EmbeddedMonitorPort: got %d, want 8222", got)
	}
}

// TestLoad_EmbeddedNATSStartTimeoutOverride verifies that
// NATS_EMBEDDED_START_TIMEOUT (Go duration syntax) plumbs through.
func TestLoad_EmbeddedNATSStartTimeoutOverride(t *testing.T) {
	t.Setenv("NATS_MODE", "embedded")
	t.Setenv("NATS_EMBEDDED_START_TIMEOUT", "30s")
	os.Unsetenv("MIGRATOR_PLATFORM_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.NATS.EmbeddedStartTimeout; got != 30*time.Second {
		t.Errorf("EmbeddedStartTimeout: got %v, want 30s", got)
	}
}

// TestLoad_RejectsBadNATSMode covers the same DB_MODE typo guard for
// NATS_MODE. Silent fall-through to "external" on a typo violates
// binary-modes.yml § no_silent_mode_changes.
func TestLoad_RejectsBadNATSMode(t *testing.T) {
	t.Setenv("NATS_MODE", "embeddd")

	cfg, err := Load()
	if err == nil {
		t.Fatalf("expected error for NATS_MODE=embeddd, got cfg=%+v", cfg)
	}
	if !strings.Contains(err.Error(), "NATS_MODE") {
		t.Errorf("error should mention NATS_MODE, got: %v", err)
	}
}

// TestLoad_RejectsBadNATSPort covers the out-of-range port guard for
// NATS_EMBEDDED_PORT. Same reasoning as DB_EMBEDDED_PORT — typos like
// 70000 must fail at startup, not silently wrap or bind to a wrong
// port.
func TestLoad_RejectsBadNATSPort(t *testing.T) {
	cases := []struct {
		name string
		port string
	}{
		{"zero", "0"},
		{"negative", "-1"},
		{"too-large", "70000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NATS_MODE", "embedded")
			t.Setenv("NATS_EMBEDDED_PORT", tc.port)
			os.Unsetenv("MIGRATOR_PLATFORM_ENABLED")

			cfg, err := Load()
			if err == nil {
				t.Fatalf("expected error for NATS_EMBEDDED_PORT=%s, got cfg=%+v", tc.port, cfg)
			}
			if !strings.Contains(err.Error(), "NATS_EMBEDDED_PORT") {
				t.Errorf("error should mention NATS_EMBEDDED_PORT, got: %v", err)
			}
		})
	}
}

// TestLoad_RejectsBadNATSMonitorPort verifies that an out-of-range
// monitor port (when opted in) is rejected, but that the 0 sentinel
// (disabled, the spec default) is NOT — those are exercised by other
// tests above.
func TestLoad_RejectsBadNATSMonitorPort(t *testing.T) {
	t.Setenv("NATS_MODE", "embedded")
	t.Setenv("NATS_EMBEDDED_MONITOR_PORT", "70000")
	os.Unsetenv("MIGRATOR_PLATFORM_ENABLED")

	cfg, err := Load()
	if err == nil {
		t.Fatalf("expected error for NATS_EMBEDDED_MONITOR_PORT=70000, got cfg=%+v", cfg)
	}
	if !strings.Contains(err.Error(), "NATS_EMBEDDED_MONITOR_PORT") {
		t.Errorf("error should mention NATS_EMBEDDED_MONITOR_PORT, got: %v", err)
	}
}

// TestLoad_RejectsEmbeddedNATSWithMultitenant verifies the multitenant
// constraint from binary-modes.yml § nats_jetstream.multitenant_constraint:
// embedded NATS does not support operator-JWT, so the combination of
// NATS_MODE=embedded + MIGRATOR_PLATFORM_ENABLED=true must be refused
// at startup with a clear pointer at the resolution.
func TestLoad_RejectsEmbeddedNATSWithMultitenant(t *testing.T) {
	t.Setenv("NATS_MODE", "embedded")
	t.Setenv("MIGRATOR_PLATFORM_ENABLED", "true")

	cfg, err := Load()
	if err == nil {
		t.Fatalf("expected error for embedded NATS + multitenant, got cfg=%+v", cfg)
	}
	// The error message should mention the actual remediation paths so
	// an operator immediately knows whether to flip NATS_MODE or unset
	// MIGRATOR_PLATFORM_ENABLED.
	for _, want := range []string{"NATS_MODE=external", "MIGRATOR_PLATFORM_ENABLED"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// TestLoad_ExternalNATSWithMultitenantAllowed is the defensive inverse:
// the multitenant guard is mode-specific, NOT a blanket prohibition.
// External NATS + MIGRATOR_PLATFORM_ENABLED=true must continue to work
// (it's the canonical platform-mode deployment shape).
func TestLoad_ExternalNATSWithMultitenantAllowed(t *testing.T) {
	t.Setenv("NATS_MODE", "external")
	t.Setenv("MIGRATOR_PLATFORM_ENABLED", "true")
	t.Setenv("NATS_URL", "nats://prod-nats:4222")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.NATS.Mode; got != "external" {
		t.Errorf("Mode: got %q, want external", got)
	}
	if got := cfg.NATS.URL; got != "nats://prod-nats:4222" {
		t.Errorf("URL: got %q, want nats://prod-nats:4222 (external mode preserves NATS_URL)", got)
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
