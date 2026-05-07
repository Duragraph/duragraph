// Command duragraph is the canonical entrypoint for the DuraGraph
// control plane binary. The cobra command tree lives in the cmd
// subpackage; main.go is intentionally tiny — all behaviour is in
// the subcommands.
package main

import (
	"os"

	"github.com/duragraph/duragraph/cmd/duragraph/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Cobra has already printed the error to stderr (rootCmd has
		// SilenceUsage=true so the usage block is suppressed; the
		// error message itself is still printed). Non-zero exit so
		// callers / shells can detect failure.
		os.Exit(1)
	}
}
