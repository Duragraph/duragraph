package cmd

import "github.com/spf13/cobra"

// initCmd scaffolds a new duragraph project. Stub for now — template
// pull-from-duragraph-examples wiring lands in phase 6 of the v0.7
// single-binary DX track (binary-modes.yml § migration.phasing.phase_6_init_template).
var initCmd = &cobra.Command{
	Use:   "init <project-name>",
	Short: "Scaffold a new duragraph project",
	Long: `Create a new duragraph project directory with starter agent code, project
config, and run instructions.

Templates: hello-world | chatbot | rag | tool-use (pulled from duragraph-examples/python).

Not yet implemented. Tracking phase 6 of binary-modes.yml § migration.phasing.`,
	RunE: notYetImplemented("init"),
}

func init() {
	rootCmd.AddCommand(initCmd)
}
