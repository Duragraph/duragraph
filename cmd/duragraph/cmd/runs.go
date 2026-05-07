package cmd

import "github.com/spf13/cobra"

// runsCmd is a subcommand group, not a leaf — `duragraph runs` with
// no further argument should show the help block listing tail/get/
// trigger. cobra does this by default when a parent command has no
// Run / RunE.
var runsCmd = &cobra.Command{
	Use:   "runs",
	Short: "CLI access to the runs API (stub — not yet implemented)",
	Long: `Inspect, stream, and trigger runs via the duragraph control plane API.

Not yet implemented. Tracking phase 7 of binary-modes.yml § migration.phasing.phase_7_runs_events_tail.`,
}

var runsTailCmd = &cobra.Command{
	Use:   "tail [<thread_id>]",
	Short: "Stream new runs across all threads (or a single thread)",
	Long: `Subscribe to the run-lifecycle SSE stream and print events as they happen.

Not yet implemented.`,
	RunE: notYetImplemented("runs tail"),
}

var runsGetCmd = &cobra.Command{
	Use:   "get <run_id>",
	Short: "Print a run's current state and output as JSON",
	Long: `Fetch a single run by ID and print the full state + output as JSON.

Not yet implemented.`,
	RunE: notYetImplemented("runs get"),
}

var runsTriggerCmd = &cobra.Command{
	Use:   "trigger <assistant_id>",
	Short: "Create a one-shot run against an assistant",
	Long: `Create a stateless run with the given input. With --wait, block until
the run reaches a terminal state and print the final output.

Not yet implemented.`,
	RunE: notYetImplemented("runs trigger"),
}

func init() {
	runsCmd.AddCommand(runsTailCmd, runsGetCmd, runsTriggerCmd)
	rootCmd.AddCommand(runsCmd)
}
