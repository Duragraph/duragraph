package cmd

import "github.com/spf13/cobra"

// eventsCmd is a subcommand group like `runs`. `events tail` is the
// only child today; future PRs may add `events search` etc.
var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "CLI access to the event sourcing trail",
	Long: `Inspect the immutable event log via NATS subscription.

Not yet implemented. Tracking phase 7 of binary-modes.yml § migration.phasing.phase_7_runs_events_tail.`,
}

var eventsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Live-tail the events table via NATS",
	Long: `Subscribe to the event sourcing trail and print event_type + payload as
they arrive. Optional filtering by aggregate type (run|thread|workflow) and id.

Not yet implemented.`,
	RunE: notYetImplemented("events tail"),
}

func init() {
	eventsCmd.AddCommand(eventsTailCmd)
	rootCmd.AddCommand(eventsCmd)
}
