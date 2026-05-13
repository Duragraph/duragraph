package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
)

// DefaultNATSURL is the NATS URL the CLI assumes when neither the --nats
// flag nor the DURAGRAPH_NATS_URL environment variable is set. Mirrors
// the embedded-NATS port the `dev` command starts on (binary-modes.yml
// § subcommands.dev) — defaultNATSEmbeddedPort in internal/config/config.go.
// Operators connecting to a canonical/external NATS at 4222 should pass
// --nats nats://localhost:4222 or set DURAGRAPH_NATS_URL.
const DefaultNATSURL = "nats://localhost:14222"

// NATS subject mapping for `events tail --aggregate` filtering. Mirrors
// the routing in internal/infrastructure/messaging/outbox_relay.go's
// buildTopic — keep these in sync. Any new aggregate-type that gets
// routed to a non-`events` category in outbox_relay.go must be added
// here as well.
//
// outbox_relay.buildTopic produces:
//
//	aggregateType="run"        → duragraph.runs.run.<event>
//	aggregateType="execution"  → duragraph.executions.execution.<event>
//	aggregateType=<other>      → duragraph.events.<other>.<event>
//
// We deliberately match the FULL three-segment prefix (e.g.
// `duragraph.runs.run.>`) rather than the looser `duragraph.runs.>` so
// the CLI doesn't accidentally pick up unrelated traffic that other
// engine subsystems publish under `duragraph.runs.*` — notably the
// task-queue (internal/infrastructure/messaging/nats/task_queue.go,
// `duragraph.runs.<runID>.<eventType>`) and the per-run SSE bridge
// (internal/infrastructure/streaming/bridge.go, `duragraph.stream.*`).
//
// Unfiltered subscription uses `duragraph.>` so all subject families
// (events / runs / executions / stream / tasks) are covered.
const (
	subjectAll        = "duragraph.>"
	subjectRunsAll    = "duragraph.runs.run.>"
	subjectExecAll    = "duragraph.executions.execution.>"
	subjectEventsRoot = "duragraph.events"
)

// SubjectFor returns the NATS wildcard subject the CLI subscribes to
// for the given --aggregate filter. Empty filter ("") returns the
// catch-all `duragraph.>`. An unknown filter is an error rather than a
// silent fall-through to the catch-all so typos don't quietly produce
// "subscribed but never receive anything".
func SubjectFor(aggregate string) (string, error) {
	switch aggregate {
	case "":
		return subjectAll, nil
	case "run":
		return subjectRunsAll, nil
	case "execution":
		return subjectExecAll, nil
	case "thread", "workflow", "tenant", "user":
		return subjectEventsRoot + "." + aggregate + ".>", nil
	default:
		return "", fmt.Errorf("unknown aggregate filter %q (expected one of: run, execution, thread, workflow, tenant, user)", aggregate)
	}
}

// EventEnvelope mirrors the JSON shape the OutboxRelay publishes to
// NATS (see internal/infrastructure/messaging/outbox_relay.go's
// publishMessage). Fields use `any` for forward-compat — the CLI does
// not depend on payload schema and just pretty-prints it.
//
// Wire format: raw JSON in the NATS message Data. The event's stable
// ID (msg.EventID) also rides as the `Nats-Msg-Id` header for
// JetStream dedup, but the CLI doesn't need that — `json.Unmarshal`
// against msg.Data yields a fully-populated envelope.
type EventEnvelope struct {
	EventID       string         `json:"event_id"`
	AggregateType string         `json:"aggregate_type"`
	AggregateID   string         `json:"aggregate_id"`
	EventType     string         `json:"event_type"`
	Payload       any            `json:"payload"`
	Metadata      map[string]any `json:"metadata"`
	Timestamp     any            `json:"timestamp"`
}

// SubscribeEvents connects to NATS at natsURL, subscribes to subject,
// and invokes fn for each decoded event envelope. Blocks until ctx is
// cancelled. Optional aggregateID filters messages client-side (since
// thread/workflow IDs are not part of the NATS subject hierarchy).
//
// Wire format: post-watermill, the publisher writes the EventEnvelope
// as raw JSON in NATS msg.Data with `Nats-Msg-Id` (+ optional metadata)
// in the headers. No gob envelope — `json.Unmarshal(m.Data, &env)` is
// the whole decode path.
//
// Core NATS vs JetStream: JetStream-published messages also fan out to
// plain core-NATS subscribers (it's just a stream layered on top of
// pub/sub). Using core nc.Subscribe avoids leaving durable consumer
// state on the broker every time someone runs `events tail`.
func SubscribeEvents(ctx context.Context, natsURL, subject, aggregateID string, fn func(EventEnvelope) error) error {
	if natsURL == "" {
		return errors.New("SubscribeEvents: natsURL is required")
	}
	if subject == "" {
		return errors.New("SubscribeEvents: subject is required")
	}

	nc, err := natsgo.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connect NATS %s: %w", natsURL, err)
	}
	defer nc.Drain()

	// Buffered errCh so a slow caller-side fn doesn't block the NATS
	// dispatcher goroutine. The first error wins and unblocks the
	// outer select.
	errCh := make(chan error, 1)

	sub, err := nc.Subscribe(subject, func(m *natsgo.Msg) {
		// JSON → EventEnvelope. Be lenient — wildcards on
		// `duragraph.>` may catch non-envelope traffic that other
		// engine subsystems publish; just drop undecodable frames.
		var env EventEnvelope
		if err := json.Unmarshal(m.Data, &env); err != nil {
			return
		}
		// Optional client-side aggregate-id filter.
		if aggregateID != "" && env.AggregateID != aggregateID {
			return
		}
		if err := fn(env); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe %q on %s: %w", subject, natsURL, err)
	}
	defer sub.Unsubscribe()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}
