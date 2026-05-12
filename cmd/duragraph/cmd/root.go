// Package cmd holds the cobra command tree for the duragraph binary.
//
// Foundation for the v0.7 single-binary DX track (see
// duragraph-spec/backend/binary-modes.yml). This PR delivers only the
// scaffolding plus `serve` and `version`. The other subcommands —
// `dev`, `init`, `migrate`, `runs`, `events`, `studio` — are stubs
// that exit 1 with a "not yet implemented" message; their bodies will
// be filled in by follow-up PRs.
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/duragraph/duragraph/internal/pkg/logging"
)

// Persistent flag values, populated by cobra. Hooked into PersistentPreRun
// below so they're applied before any subcommand RunE fires.
var (
	logFormat string
	logLevel  string
)

// rootCmd is the top-level `duragraph` command. With no subcommand
// supplied, rootCmd defaults to the `serve` behaviour for backwards
// compatibility with the previous `cmd/server` entrypoint (which had
// no subcommand surface — invoking the binary just started the
// server).
var rootCmd = &cobra.Command{
	Use:   "duragraph",
	Short: "DuraGraph control plane binary",
	Long: `duragraph is the control plane binary for the DuraGraph platform.
Run with no subcommand to start the server (equivalent to "duragraph serve").`,
	SilenceUsage: true,
	// PersistentPreRun is the only cobra hook guaranteed to fire for
	// every subcommand. Wire slog.Default() here so by the time any
	// RunE body runs, slog calls hit the right handler.
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		// Env vars are a fallback when the flag isn't set (cobra
		// flag defaults run before this hook, so we can't distinguish
		// "user passed --log-format=auto" from "default"; the env
		// fallback only fires when the resolved Format is Auto).
		format := logging.ParseFormat(logFormat)
		if format == logging.FormatAuto {
			if env := os.Getenv("DURAGRAPH_LOG_FORMAT"); env != "" {
				format = logging.ParseFormat(env)
			}
		}
		level := logging.ParseLevelString(logLevel)
		if logLevel == "" {
			if env := os.Getenv("DURAGRAPH_LOG_LEVEL"); env != "" {
				level = logging.ParseLevelString(env)
			}
		}
		logging.Setup(format, level)
	},
}

// Execute runs the root cobra command. main() in cmd/duragraph/main.go
// (and the back-compat shim cmd/server/main.go) call this and rely on
// cobra to print errors + own the exit code.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Persistent flags apply to every subcommand.
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "auto",
		"log output format: auto | pretty | json (env: DURAGRAPH_LOG_FORMAT)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"log level: debug | info | warn | error (env: DURAGRAPH_LOG_LEVEL)")

	// Backwards compat: bare `duragraph` (no subcommand) behaves like
	// `duragraph serve`. Wired here rather than in serve.go's init so
	// the assignment happens exactly once and is unambiguous about
	// where the default lives.
	rootCmd.RunE = serveCmd.RunE
}
