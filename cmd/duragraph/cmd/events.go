package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/duragraph/duragraph/internal/dev/cli"
	"github.com/spf13/cobra"
)

// Phase 7 (binary-modes.yml § subcommands.duragraph_events): live-tail
// the event-sourcing trail via NATS subscription.
//
// The CLI subscribes via core NATS (not JetStream) because the engine
// publishes JetStream-backed messages that also fan out to plain
// subscribers. Going through plain pub/sub means short-lived `events
// tail` invocations don't leave durable consumer state on the broker —
// matches the "live-only, best-effort" expectation for an interactive
// inspector.

var (
	eventsTailNATSURL     string
	eventsTailAggregate   string
	eventsTailAggregateID string
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Inspect the event-sourcing trail",
	Long: `events talks to NATS to live-tail or query the engine's domain event
log. Phase 7 ships only the "tail" subcommand; future phases will add
search/replay surfaces.`,
}

var eventsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Live-tail the events table via NATS subscription",
	Long: `Subscribe to the engine's NATS event subject and pretty-print each
event envelope (event_type + payload) as it is published.

Filter by aggregate type with --aggregate (run | execution | thread |
workflow | tenant | user). Filter further by aggregate id with --id.

The NATS URL defaults to nats://localhost:4222 and is overridable via
$DURAGRAPH_NATS_URL or --nats. Press Ctrl+C to stop.`,
	RunE: runEventsTail,
}

func init() {
	eventsTailCmd.Flags().StringVar(&eventsTailNATSURL, "nats", "",
		"NATS URL (defaults to $DURAGRAPH_NATS_URL or "+cli.DefaultNATSURL+")")
	eventsTailCmd.Flags().StringVar(&eventsTailAggregate, "aggregate", "",
		"Filter by aggregate type: run | execution | thread | workflow | tenant | user")
	eventsTailCmd.Flags().StringVar(&eventsTailAggregateID, "id", "",
		"Filter by aggregate id (applied client-side after subject filter)")

	eventsCmd.AddCommand(eventsTailCmd)
	rootCmd.AddCommand(eventsCmd)
}

func runEventsTail(cmd *cobra.Command, _ []string) error {
	subject, err := cli.SubjectFor(eventsTailAggregate)
	if err != nil {
		return err
	}

	natsURL := eventsTailNATSURL
	if natsURL == "" {
		if v := os.Getenv("DURAGRAPH_NATS_URL"); v != "" {
			natsURL = v
		} else {
			natsURL = cli.DefaultNATSURL
		}
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	out := cmd.OutOrStdout()
	fmt.Fprintf(cmd.ErrOrStderr(), "tailing events via NATS %s subject=%s", natsURL, subject)
	if eventsTailAggregateID != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), " id=%s", eventsTailAggregateID)
	}
	fmt.Fprintln(cmd.ErrOrStderr())

	err = cli.SubscribeEvents(ctx, natsURL, subject, eventsTailAggregateID, func(env cli.EventEnvelope) error {
		return cli.PrintEvent(out, env.EventType, env)
	})
	if err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
