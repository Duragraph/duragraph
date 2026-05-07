package cmd

import "github.com/spf13/cobra"

// studioCmd manages the bundled Studio UI dist on disk. install
// downloads + unpacks; uninstall removes. Both stubs today —
// implementation lands in phase 8 of binary-modes.yml § migration.
var studioCmd = &cobra.Command{
	Use:   "studio",
	Short: "Manage the bundled Studio UI",
	Long: `Install or uninstall the optional Studio dist bundle. Studio is opt-in —
the binary itself does not embed Studio (Studio versions independently and
inflating the binary for users who don't want it is rejected on size grounds).

Not yet implemented. Tracking phase 8 of binary-modes.yml § migration.phasing.phase_8_studio_bundling.`,
}

var studioInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download and install the Studio UI",
	Long: `Download the Studio dist tarball (release asset on duragraph-studio) and
unpack it under the data dir.

Not yet implemented.`,
	RunE: notYetImplemented("studio install"),
}

var studioUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the installed Studio UI",
	Long: `Remove the Studio dist files from the data dir.

Not yet implemented.`,
	RunE: notYetImplemented("studio uninstall"),
}

func init() {
	studioCmd.AddCommand(studioInstallCmd, studioUninstallCmd)
	rootCmd.AddCommand(studioCmd)
}
