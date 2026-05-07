package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

// devCmd is the zero-config dev mode entrypoint
// (binary-modes.yml § subcommands.duragraph_dev).
//
// Implementation strategy: dev does NOT duplicate serve's ~1100 line
// startup body. It pre-sets a small set of environment variables that
// flip serve into the "embedded postgres + embedded NATS, single-tenant,
// no auth" configuration, then delegates to serveCmd.RunE. This keeps
// the engine startup path single-sourced — every change to serve
// (new wiring, new metric, new handler) automatically benefits dev.
//
// Phase scope (v0.7):
//   - Phase 4 (this PR): wire dev to embedded mode, accept --watch and
//     --studio as no-op flags with warnings.
//   - Phase 5: implement --watch (file-watching worker supervision).
//   - Phase 8: implement --studio (serve bundled Studio under /studio/).
//
// TODO(post-v0.7): human-readable log format. serve currently emits
// JSON logs by default; dev inherits that. The spec calls for "color,
// timestamps, structured" stdout in dev. Wiring a logger toggle
// requires a serve-side refactor (the current logger is constructed
// inline inside RunE) and is deferred to its own PR.
var (
	devPort    int
	devDataDir string
	devWatch   string
	devStudio  bool

	devCmd = &cobra.Command{
		Use:   "dev",
		Short: "Run duragraph with embedded postgres, nats, and watch mode",
		Long: `Zero-config dev mode: embedded Postgres + embedded NATS + dashboard
+ optional Studio + worker watch mode.

Defaults to a single-tenant deployment with auth disabled — designed for
the local-laptop demo path described in binary-modes.yml § subcommands.

Operator overrides win: setting DB_MODE=external (or any of the other
env vars listed below) before invoking "duragraph dev" preserves your
choice. The flag-derived defaults only fire when the env var is unset.`,
		RunE: runDev,
	}
)

func init() {
	devCmd.Flags().IntVar(&devPort, "port", 8081, "HTTP port for the engine + dashboard")
	devCmd.Flags().StringVar(&devDataDir, "data-dir", "./data",
		"Data directory for embedded postgres + NATS storage. Created on first run.")
	devCmd.Flags().StringVar(&devWatch, "watch", "./agents",
		"Directory to watch for graph definitions (Phase 5 — currently a no-op)")
	devCmd.Flags().BoolVar(&devStudio, "studio", false,
		"Serve bundled Studio at /studio/ (Phase 8 — currently a no-op)")

	rootCmd.AddCommand(devCmd)
}

// runDev sets dev-mode env defaults, prints the banner, and hands off
// to serveCmd. Splitting the env-mutation step into applyDevEnvDefaults
// keeps that logic unit-testable without standing up the full engine.
func runDev(cmd *cobra.Command, args []string) error {
	absDataDir, err := filepath.Abs(devDataDir)
	if err != nil {
		return fmt.Errorf("resolve --data-dir: %w", err)
	}
	if err := os.MkdirAll(absDataDir, 0o755); err != nil {
		return fmt.Errorf("create --data-dir %s: %w", absDataDir, err)
	}

	applyDevEnvDefaults(devOptions{
		Port:    devPort,
		DataDir: absDataDir,
	})

	fmt.Println("🚀 duragraph dev — embedded postgres + nats, single-tenant")
	fmt.Printf("   data dir: %s\n", absDataDir)
	fmt.Printf("   dashboard: http://localhost:%d/\n", devPort)

	// Phase 5/8 stubs: accept the flag, warn, continue. The flag surface
	// stays stable so subsequent phases don't break invocations baked
	// into operator scripts. --watch has a non-empty default
	// (./agents) so the warning fires unconditionally — the operator
	// should know watch isn't wired regardless of whether they set the
	// flag explicitly.
	fmt.Printf("⚠️  watch mode not yet implemented (Phase 5); --watch=%s ignored\n", devWatch)
	if devStudio {
		fmt.Println("⚠️  studio bundling not yet implemented (Phase 8); --studio ignored")
	}

	// We pass dev's cmd + args straight through. Today serveCmd.RunE
	// (runServe) ignores both — see its signature `_ *cobra.Command,
	// _ []string`. If serve ever starts inspecting cmd.Flags(), this
	// call site needs revisiting (dev's flag set != serve's).
	return serveCmd.RunE(cmd, args)
}

// devOptions captures the flag values that translate into env defaults.
// Pulled into a struct so applyDevEnvDefaults stays a pure function for
// tests (no globals, no flag package state).
type devOptions struct {
	Port    int
	DataDir string
}

// applyDevEnvDefaults sets the env vars that flip the engine into
// embedded-mode dev defaults. Uses setIfUnset so an operator who
// explicitly set, e.g., DB_MODE=external before invoking dev keeps
// pointing at their external DB.
func applyDevEnvDefaults(opts devOptions) {
	setIfUnset("DB_MODE", "embedded")
	setIfUnset("NATS_MODE", "embedded")
	setIfUnset("MIGRATOR_PLATFORM_ENABLED", "false")
	setIfUnset("AUTH_ENABLED", "false")
	setIfUnset("PORT", strconv.Itoa(opts.Port))
	setIfUnset("DB_EMBEDDED_DATA_DIR", filepath.Join(opts.DataDir, "pg"))
	setIfUnset("NATS_EMBEDDED_DATA_DIR", filepath.Join(opts.DataDir, "nats"))
}
