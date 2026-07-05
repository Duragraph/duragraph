// Package nats declares the JetStream topology (streams + durable
// consumers) for the control-plane rebuild and the outbox relay that
// bridges Postgres writes to NATS publishes.
//
// Source of truth: spec/models/d2/nats.d2 (streams + consumers) and
// spec/models/d2/relay.d2 (relay behavior). The d2 stays the human
// spec — this file is its machine form. Regenerate alongside any d2
// change to the topology.
//
// All streams live under the duragraph.* subject namespace. Subjects
// use the canonical event-type names from endpoint-queries.d2 (run.*,
// execution.node_*, interrupt.*, worker.*, user.*, tenant.*) so the
// relay's subject builder (publish.go) composes them by category:
//
//	duragraph.runs.run.created                       → stream RUNS
//	duragraph.executions.execution.node_completed    → stream EXECUTION
//	duragraph.interrupts.interrupt.created            → stream INTERRUPTS
//	duragraph.worker_commands.worker.graph.execute   → stream WORKER_COMMANDS
//	duragraph.platform_users.user.signed_up           → stream PLATFORM_USERS
//	duragraph.platform_tenants.tenant.provisioning    → stream PLATFORM_TENANTS
package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// dedupWindow is how far back JetStream keeps Nats-Msg-Id values for
// dedup. The outbox relay has no exponential backoff (the LISTEN wakes
// it on every commit), so any reasonable window covers retries. 2 min
// is the idiomatic JetStream default; set per-stream via
// StreamConfig.Duplicates.
const dedupWindow = 2 * time.Minute

// streamSpec is one of the six duragraph JetStream streams declared in
// nats.d2. Subjects use the canonical "duragraph.<family>.>" wildcard;
// every message published must fall under one of these streams.
type streamSpec struct {
	name     string
	subjects []string
}

// tenantStreams are the per-tenant JetStream streams (nats.d2
// tenant_account). One tenant-account is created per approved tenant,
// and these streams live inside it. Subject families use the
// duragraph.<category>.> wildcard so multi-segment event types
// (execution.node_failed, worker.graph.execute) all match.
var tenantStreams = []streamSpec{
	{name: "RUNS", subjects: []string{"duragraph.runs.>"}},
	{name: "EXECUTION", subjects: []string{"duragraph.executions.>"}},
	{name: "INTERRUPTS", subjects: []string{"duragraph.interrupts.>"}},
	{name: "WORKER_COMMANDS", subjects: []string{"duragraph.worker_commands.>"}},
}

// systemStreams are the singleton system-account streams (nats.d2
// system_account): user.* + tenant.* events for platform provisioning
// + audit consumers.
var systemStreams = []streamSpec{
	{name: "PLATFORM_USERS", subjects: []string{"duragraph.platform_users.>"}},
	{name: "PLATFORM_TENANTS", subjects: []string{"duragraph.platform_tenants.>"}},
}

// allStreams returns the union of tenant + system streams so
// EnsureStreams can declare them in one pass.
func allStreams() []streamSpec {
	out := make([]streamSpec, 0, len(tenantStreams)+len(systemStreams))
	out = append(out, tenantStreams...)
	out = append(out, systemStreams...)
	return out
}

// EnsureStreams idempotently creates or updates the six duragraph
// JetStream streams with the dedup window configured. Safe to call on
// every relay/publisher start. Existing streams keep their stored
// messages; only the subject list + dedup window are reconciled.
func EnsureStreams(ctx context.Context, js jetstream.JetStream) error {
	if js == nil {
		return errors.New("nats: nil JetStream context")
	}
	for _, s := range allStreams() {
		_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:       s.name,
			Subjects:   s.subjects,
			Storage:    jetstream.FileStorage,
			Replicas:   1,
			Duplicates: dedupWindow,
		})
		if err != nil {
			return fmt.Errorf("stream %q: %w", s.name, err)
		}
	}
	return nil
}

// Connect opens a NATS connection and returns a JetStream context
// backed by it. Both are owned by the caller; close via the returned
// Conn's Drain.
func Connect(ctx context.Context, natsURL string) (*nats.Conn, jetstream.JetStream, error) {
	if natsURL == "" {
		return nil, nil, errors.New("nats: URL is required")
	}
	conn, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("nats: connect: %w", err)
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("nats: jetstream context: %w", err)
	}
	if err := EnsureStreams(ctx, js); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("nats: ensure streams: %w", err)
	}
	return conn, js, nil
}
