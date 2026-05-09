package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// studioCmd is a thin wrapper kept for backwards-compat with the
// binary-modes.yml § subcommands.duragraph_studio surface. The
// install / uninstall actions are no-ops post-monorepo: Studio is
// now embedded into the binary at build time (see studio_embed.go's
// //go:embed directive). Operators serve it via `duragraph dev --studio`
// or `duragraph serve` with DURAGRAPH_DEV_STUDIO=true; nothing to
// download or remove.
//
// The subcommands print a brief explanation + exit 0. They are NOT
// removed entirely so that users following older docs / tutorials
// don't get a "command not found" surprise.
var studioCmd = &cobra.Command{
	Use:   "studio",
	Short: "(no-op since v0.7) Studio is now embedded at build time",
	Long: `Studio is the developer/end-user UI for interacting with deployed
agents (chat, HITL approval, run inspector). As of v0.7 the Studio bundle
is embedded into the duragraph binary at build time (the "studio/" subtree
of the monorepo is built and embedded via //go:embed).

Use ` + "`duragraph dev --studio`" + ` to mount it at /studio/, or
DURAGRAPH_DEV_STUDIO=true on ` + "`duragraph serve`" + `. The install /
uninstall subcommands are kept as no-ops for backwards compatibility
with older docs.`,
}

var studioInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "(no-op since v0.7) Studio is embedded at build time",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Studio is now embedded into the duragraph binary at build time.")
		fmt.Println("Run with --studio (dev) or DURAGRAPH_DEV_STUDIO=true (serve) to mount it.")
		fmt.Println("No download / install step is needed.")
		return nil
	},
}

var studioUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "(no-op since v0.7) Studio is embedded at build time",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Studio is embedded into the duragraph binary; there's nothing to uninstall.")
		fmt.Println("To run a build without Studio, omit --studio (and unset DURAGRAPH_DEV_STUDIO).")
		return nil
	},
}

func init() {
	studioCmd.AddCommand(studioInstallCmd, studioUninstallCmd)
	rootCmd.AddCommand(studioCmd)
}
