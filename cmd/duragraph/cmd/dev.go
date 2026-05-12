package cmd

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/duragraph/duragraph/internal/dev/watch"
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
//   - Phase 4: wire dev to embedded mode, accept --watch and --studio
//     as no-op flags with warnings.
//   - Phase 5 (this PR): implement --watch (file-watching worker
//     supervision via internal/dev/watch).
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

	devCmd = &cobra.Command{
		Use:   "dev",
		Short: "Run duragraph with embedded postgres, nats, and watch mode",
		Long: `Zero-config dev mode: embedded Postgres + embedded NATS + dashboard
+ worker watch mode.

Studio's developer-UI surface (chat playground, workflow builder,
deployments, run inspector) now lives inside the dashboard itself under
the Playground section of the sidebar — there is no separate Studio
mount point. The --studio flag accepts a value silently for
backwards-compat but is otherwise a no-op.

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
		"Directory to watch (recursively) for Python graph files; pass empty string to disable")
	// --studio kept as a deprecated no-op so existing scripts that pass
	// it don't error. The studio UI is now folded into the dashboard
	// under /playground, /builder, /deployments, /inspector. Hidden from
	// help to discourage new usage.
	var devStudioCompat bool
	devCmd.Flags().BoolVar(&devStudioCompat, "studio", false,
		"Deprecated no-op — studio is now folded into the dashboard.")
	_ = devCmd.Flags().MarkHidden("studio")

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

	slog.Info("duragraph dev — embedded postgres + nats, single-tenant",
		"data_dir", absDataDir,
		"dashboard_url", fmt.Sprintf("http://localhost:%d/", devPort),
	)

	// --studio is a deprecated no-op (studio is now folded into the
	// dashboard). Nothing to print.

	// Phase 5: --watch supervises Python graph workers under devWatch.
	// We have to start the watcher BEFORE serveCmd.RunE because runServe
	// blocks until SIGINT/SIGTERM — there's no return-after-startup hook.
	// The watcher's first action is to poll http://localhost:<port>/health,
	// so it intentionally idles until the engine is up. When serve
	// returns (cobra command finishes), we cancel the watcher's context
	// and wait for it to drain.
	//
	// Don't refactor serve.go for this — the goroutine pattern has the
	// virtue of leaving the engine startup path untouched and of letting
	// us add Phase 5 entirely behind the --watch flag.
	watcherCtx, cancelWatcher := context.WithCancel(cmd.Context())
	defer cancelWatcher()

	var watcherDone chan struct{}
	if devWatch != "" {
		w, werr := watch.New(watch.Options{
			WatchDir:   devWatch,
			EnginePort: devPort,
			Stdout:     os.Stdout,
			Stderr:     os.Stderr,
			Logger:     log.Default(),
		})
		if werr != nil {
			return fmt.Errorf("watch init: %w", werr)
		}
		watcherDone = make(chan struct{})
		go func() {
			defer close(watcherDone)
			if err := w.Run(watcherCtx); err != nil {
				slog.Error("watch supervisor failed", "err", err)
			}
		}()
	}

	// We pass dev's cmd + args straight through. Today serveCmd.RunE
	// (runServe) ignores both — see its signature `_ *cobra.Command,
	// _ []string`. If serve ever starts inspecting cmd.Flags(), this
	// call site needs revisiting (dev's flag set != serve's).
	serveErr := serveCmd.RunE(cmd, args)

	// Engine has shut down — cancel the watcher and wait for it to
	// drain (kills its supervised subprocesses cleanly). Without this
	// step, the binary returns to the operator's shell while child
	// uv/python processes still hold the terminal.
	cancelWatcher()
	if watcherDone != nil {
		<-watcherDone
	}
	return serveErr
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
