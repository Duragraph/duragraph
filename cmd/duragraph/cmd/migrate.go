package cmd

import "github.com/spf13/cobra"

// migrateCmd is explicit migration control (vs the implicit on-startup
// flow already running inside `serve`). Stub for now — useful in
// serve / multitenant ops debugging once it's filled in.
var migrateCmd = &cobra.Command{
	Use:   "migrate [up|down|status]",
	Short: "Run or inspect database migrations",
	Long: `Explicit migration control for ops debugging — separate from the implicit
on-startup migration that "serve" performs.

Not yet implemented. See duragraph-spec/backend/binary-modes.yml § subcommands.duragraph_migrate.`,
	RunE: notYetImplemented("migrate"),
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
