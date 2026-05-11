package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// studioCmd is a deprecated no-op kept so that users following older
// docs / tutorials don't get a "command not found" surprise. As of
// the studio-into-dashboard merge, studio's developer-UI surface
// (chat playground, workflow builder, deployments, run inspector) is
// folded into the dashboard's TanStack Router tree at /playground,
// /builder, /deployments, /inspector. There is no separate Studio
// binary, install step, or /studio/* mount.

var studioCmd = &cobra.Command{
	Use:    "studio",
	Short:  "(removed) Studio is now folded into the dashboard",
	Hidden: true,
	Long: `Studio is no longer a separate subcommand. Its UI surface (chat
playground, workflow builder, deployments, run inspector) is folded
into the dashboard under /playground, /builder, /deployments,
/inspector. Run "duragraph dev" or "duragraph serve" and open
http://localhost:<port>/ — everything that used to be at /studio/ is
now in the sidebar.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(
			"Studio is now folded into the dashboard. " +
				"Run `duragraph dev` and open / — see the Playground section in the sidebar.",
		)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(studioCmd)
}
