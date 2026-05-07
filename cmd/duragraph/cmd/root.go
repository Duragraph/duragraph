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
	"github.com/spf13/cobra"
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

Run with no subcommand to start the server (equivalent to "duragraph serve") —
this preserves backwards compatibility with the previous entrypoint.

Subcommands:
  serve     Run the duragraph server (long-running)
  dev       Zero-config dev mode (embedded postgres + nats)   [not yet implemented]
  init      Scaffold a new duragraph project                  [not yet implemented]
  migrate   Run/inspect database migrations                   [not yet implemented]
  runs      CLI access to the runs API                        [not yet implemented]
  events    CLI access to the event sourcing trail            [not yet implemented]
  studio    Manage the bundled Studio UI                      [not yet implemented]
  version   Print binary version and build metadata`,
	SilenceUsage: true,
}

// Execute runs the root cobra command. main() in cmd/duragraph/main.go
// (and the back-compat shim cmd/server/main.go) call this and rely on
// cobra to print errors + own the exit code.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Backwards compat: bare `duragraph` (no subcommand) behaves like
	// `duragraph serve`. Wired here rather than in serve.go's init so
	// the assignment happens exactly once and is unambiguous about
	// where the default lives.
	rootCmd.RunE = serveCmd.RunE
}
