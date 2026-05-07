// Command server is a backwards-compatibility shim for the legacy
// `./cmd/server` build target. The actual cobra command tree lives at
// cmd/duragraph/cmd; this package only exists so existing Dockerfiles
// (deploy/docker/Dockerfile.api, Dockerfile.server) that build
// `./cmd/server` keep producing a working binary while we transition
// to `./cmd/duragraph` as the canonical entrypoint.
//
// Behaviour is identical to invoking `duragraph` with no subcommand:
// rootCmd's RunE is wired to serveCmd.RunE in cmd/duragraph/cmd/root.go,
// so this shim runs the server. New flags / subcommands should be
// added under cmd/duragraph; do NOT add new logic here.
package main

import (
	"os"

	"github.com/duragraph/duragraph/cmd/duragraph/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
