package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// consumerSpec is one of the seven durable JetStream consumers from
// nats.d2 (consumers shape). Each binds to a stream with a filter
// subject (when the stream carries more than one subject family) and
// an AckWait that paces redelivery.
type consumerSpec struct {
	name       string
	stream     string
	filter     string // empty ⇒ consume the whole stream
	ackWait    time.Duration
	maxDeliver int // 0 ⇒ unlimited (-1)
}

// tenantConsumers are the per-tenant durable consumers from nats.d2.
// run-processor drains RUNS; the three worker consumers filter
// WORKER_COMMANDS by the command family they handle.
var tenantConsumers = []consumerSpec{
	{name: "run-processor", stream: "RUNS", ackWait: 30 * time.Second},
	{name: "graph-executor", stream: "WORKER_COMMANDS", filter: "duragraph.worker_commands.worker.graph.execute", ackWait: 5 * time.Minute, maxDeliver: 5},
	{name: "llm-worker", stream: "WORKER_COMMANDS", filter: "duragraph.worker_commands.worker.llm.invoke", ackWait: 2 * time.Minute},
	{name: "tool-worker", stream: "WORKER_COMMANDS", filter: "duragraph.worker_commands.worker.tool.execute", ackWait: 1 * time.Minute},
}

// systemConsumers are the singleton system-account durable consumers.
// platform-provisioner drives tenant DB + NATS account creation on
// tenant.provisioning; the two audit consumers record user/tenant
// state changes for compliance.
var systemConsumers = []consumerSpec{
	{name: "platform-provisioner", stream: "PLATFORM_TENANTS", filter: "duragraph.platform_tenants.tenant.provisioning", ackWait: 5 * time.Minute},
	{name: "platform-audit-users", stream: "PLATFORM_USERS", ackWait: 30 * time.Second},
	{name: "platform-audit-tenants", stream: "PLATFORM_TENANTS", ackWait: 30 * time.Second},
}

// allConsumers returns the union of tenant + system consumers.
func allConsumers() []consumerSpec {
	out := make([]consumerSpec, 0, len(tenantConsumers)+len(systemConsumers))
	out = append(out, tenantConsumers...)
	out = append(out, systemConsumers...)
	return out
}

// EnsureConsumers idempotently creates or updates the seven durable
// JetStream consumers from nats.d2. Called once on relay startup;
// existing consumer state (delivered seq, ack floor) is preserved by
// CreateOrUpdateConsumer. AckExplicit means the consumer must ack each
// message manually — relay-driven consumers do, and worker consumers
// ack in their handler loops.
func EnsureConsumers(ctx context.Context, js jetstream.JetStream) error {
	if js == nil {
		return errors.New("nats: nil JetStream context")
	}
	for _, c := range allConsumers() {
		cfg := jetstream.ConsumerConfig{
			Durable:       c.name,
			AckPolicy:     jetstream.AckExplicitPolicy,
			AckWait:       c.ackWait,
			FilterSubject: c.filter,
		}
		if c.maxDeliver > 0 {
			cfg.MaxDeliver = c.maxDeliver
		} else {
			cfg.MaxDeliver = -1
		}
		if _, err := js.CreateOrUpdateConsumer(ctx, c.stream, cfg); err != nil {
			return fmt.Errorf("consumer %q on stream %q: %w", c.name, c.stream, err)
		}
	}
	return nil
}
