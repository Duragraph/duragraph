package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetIfUnset(t *testing.T) {
	t.Run("sets when unset", func(t *testing.T) {
		t.Setenv("DG_TEST_SETIFUNSET_A", "")
		setIfUnset("DG_TEST_SETIFUNSET_A", "wrote")
		if got := os.Getenv("DG_TEST_SETIFUNSET_A"); got != "wrote" {
			t.Fatalf("expected %q, got %q", "wrote", got)
		}
	})

	t.Run("preserves existing value", func(t *testing.T) {
		t.Setenv("DG_TEST_SETIFUNSET_B", "preset")
		setIfUnset("DG_TEST_SETIFUNSET_B", "ignored")
		if got := os.Getenv("DG_TEST_SETIFUNSET_B"); got != "preset" {
			t.Fatalf("expected %q (preserved), got %q", "preset", got)
		}
	})
}

func TestApplyDevEnvDefaults(t *testing.T) {
	t.Run("sets all defaults when unset", func(t *testing.T) {
		// Clear every var the function touches. t.Setenv("", "")
		// restores the prior value at the end of the test, so this
		// doesn't leak across subtests.
		for _, k := range []string{
			"DB_MODE", "NATS_MODE", "MIGRATOR_PLATFORM_ENABLED",
			"AUTH_ENABLED", "PORT", "DB_EMBEDDED_DATA_DIR", "NATS_EMBEDDED_DATA_DIR",
		} {
			t.Setenv(k, "")
		}
		applyDevEnvDefaults(devOptions{Port: 9999, DataDir: "/tmp/dg-dev-test"})

		want := map[string]string{
			"DB_MODE":                   "embedded",
			"NATS_MODE":                 "embedded",
			"MIGRATOR_PLATFORM_ENABLED": "false",
			"AUTH_ENABLED":              "false",
			"PORT":                      "9999",
			"DB_EMBEDDED_DATA_DIR":      filepath.Join("/tmp/dg-dev-test", "pg"),
			"NATS_EMBEDDED_DATA_DIR":    filepath.Join("/tmp/dg-dev-test", "nats"),
		}
		for k, v := range want {
			if got := os.Getenv(k); got != v {
				t.Errorf("%s: expected %q, got %q", k, v, got)
			}
		}
	})

	t.Run("respects operator overrides", func(t *testing.T) {
		// An operator pointing dev at an external DB / different port
		// must not have those choices clobbered.
		t.Setenv("DB_MODE", "external")
		t.Setenv("NATS_MODE", "external")
		t.Setenv("AUTH_ENABLED", "true")
		t.Setenv("PORT", "12345")
		t.Setenv("DB_EMBEDDED_DATA_DIR", "/custom/pg")
		t.Setenv("NATS_EMBEDDED_DATA_DIR", "/custom/nats")
		t.Setenv("MIGRATOR_PLATFORM_ENABLED", "true")

		applyDevEnvDefaults(devOptions{Port: 8081, DataDir: "/tmp/dg-dev-test"})

		preserved := map[string]string{
			"DB_MODE":                   "external",
			"NATS_MODE":                 "external",
			"AUTH_ENABLED":              "true",
			"PORT":                      "12345",
			"DB_EMBEDDED_DATA_DIR":      "/custom/pg",
			"NATS_EMBEDDED_DATA_DIR":    "/custom/nats",
			"MIGRATOR_PLATFORM_ENABLED": "true",
		}
		for k, v := range preserved {
			if got := os.Getenv(k); got != v {
				t.Errorf("%s clobbered: expected %q, got %q", k, v, got)
			}
		}
	})
}
