// Sub-worker delegation — graph-engine.d2 §8.
//
//	llm node  : graph_worker → worker.llm.invoke   → LLM Worker
//	tool node  : graph_worker → worker.tool.execute → Tool Worker
//	why        : "isolate provider latency/limits from the graph loop"
//
// Until this existed, llm and tool nodes were applied declaratively from
// node.config by configExecutor — deterministic, dependency-free, and incapable
// of calling anything. A graph could describe an LLM step but never take one.
//
// WHY REQUEST-REPLY RATHER THAN A DURABLE HAND-OFF. The graph walk needs the
// node's writes to continue: the result is not a notification, it is the return
// value of the step. So the graph worker blocks on a reply, bounded by the
// consumer's ack_wait (2m for llm, 1m for tool — controlplane/nats/consumers).
// Durability across a graph-worker crash is already provided by the layer
// above: the graph command is itself a JetStream delivery, so a lost invocation
// is redelivered and replayed from the last checkpoint rather than resumed
// mid-node. Adding a second durable queue underneath would buy nothing and
// would let a node's side effect outlive the run that asked for it.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Subjects the sub-workers listen on. These mirror nats.d2's declared subjects
// and the filters on the llm-worker / tool-worker consumers.
const (
	SubjectLLMInvoke   = "worker.llm.invoke"
	SubjectToolExecute = "worker.tool.execute"
)

// InvokeRequest is what a graph worker sends to a sub-worker. It carries the
// node's identity so a sub-worker's logs and events can be tied back to the run
// that caused them, and the channels so a provider can build its prompt from
// the graph's state.
type InvokeRequest struct {
	RunID    string         `json:"run_id"`
	NodeID   string         `json:"node_id"`
	NodeType string         `json:"node_type"`
	Config   map[string]any `json:"config"`
	Channels map[string]any `json:"channels"`
}

// InvokeResponse is the sub-worker's reply. Writes merge into channel_values
// exactly as an inline executor's would, so delegation is invisible to the
// walk. Error is a STRING rather than a Go error because it crosses the wire;
// a non-empty Error makes the node fail deterministically.
type InvokeResponse struct {
	Writes map[string]any `json:"writes"`
	Error  string         `json:"error,omitempty"`
}

// Invoker performs one request/reply round trip to a sub-worker. It is an
// interface so the graph worker can be tested without a live sub-worker, and so
// the transport (NATS today) is not baked into the executor.
type Invoker interface {
	Invoke(ctx context.Context, subject string, req InvokeRequest, timeout time.Duration) (InvokeResponse, error)
}

// delegatingExecutor turns an llm/tool node into a sub-worker call.
//
// It falls back to configExecutor when no Invoker is wired. That is not a
// convenience: a deployment with no sub-workers running would otherwise fail
// every llm node, and the declarative form is what the whole existing test
// suite is written against. The fallback keeps a graph that only uses `set`
// and `fail` working with or without a fleet.
type delegatingExecutor struct {
	subject string
	timeout time.Duration
	inv     Invoker

	// emit records the in-node observability events api.d2 declares
	// (llm.completion, tool.call, tool.result). Optional: a nil emitter means
	// the node still runs, it is just not narrated. These events must never be
	// able to fail the node that produced them.
	emit func(ctx context.Context, eventType, nodeID string, input, output json.RawMessage, durationMs *int)
}

// narrate emits an observability event if an emitter is wired, swallowing any
// failure. A node must not fail because its narration did.
func (d delegatingExecutor) narrate(ctx context.Context, eventType, nodeID string, input, output json.RawMessage, durationMs *int) {
	if d.emit == nil {
		return
	}
	d.emit(ctx, eventType, nodeID, input, output, durationMs)
}

func (d delegatingExecutor) Execute(ctx context.Context, node Node, channels map[string]any) (map[string]any, error) {
	// With no fleet wired, behave EXACTLY as before: apply node.config
	// declaratively. Erroring instead would break every graph whose llm/tool
	// node carries neither `set` nor `fail` — an interrupt gate, or a node with
	// no config at all — which previously passed through harmlessly. A
	// deployment without sub-workers must keep running the graphs it already
	// ran, not start failing them.
	if d.inv == nil {
		return configExecutor{}.Execute(ctx, node, channels)
	}
	// A node that declares deterministic behaviour keeps it even with a fleet
	// available: `fail` is how a poison node is built and `set` how a
	// predictable one is, and neither should suddenly start calling a provider.
	if _, deterministic := node.Config["fail"]; deterministic {
		return configExecutor{}.Execute(ctx, node, channels)
	}
	if _, deterministic := node.Config["set"]; deterministic {
		return configExecutor{}.Execute(ctx, node, channels)
	}

	// tool.call is emitted BEFORE the tool runs: a tool invocation is a side
	// effect on the outside world, and a caller watching the stream needs to
	// see that it was attempted even if it never returns.
	if node.Type == "tool" {
		d.narrate(ctx, "tool.call", node.ID, mustJSONRaw(node.Config), nil, nil)
	}

	startedAt := time.Now()
	resp, err := d.inv.Invoke(ctx, d.subject, InvokeRequest{
		RunID:    runIDFromContext(ctx),
		NodeID:   node.ID,
		NodeType: node.Type,
		Config:   node.Config,
		Channels: channels,
	}, d.timeout)
	elapsed := int(time.Since(startedAt).Milliseconds())
	if err == nil && resp.Error == "" {
		// llm.completion / tool.result — the provider answered.
		eventType := "llm.completion"
		if node.Type == "tool" {
			eventType = "tool.result"
		}
		d.narrate(ctx, eventType, node.ID, nil, mustJSONRaw(resp.Writes), &elapsed)
	}
	if err != nil {
		// A transport failure is NOT a poison node: the provider may simply be
		// slow or briefly unreachable, and failing the run would discard work
		// the checkpoint could have resumed. Surfacing it as an error lets the
		// runner's existing ack matrix Nak and redeliver.
		return nil, fmt.Errorf("node %s: %s sub-worker: %w", node.ID, node.Type, err)
	}
	if resp.Error != "" {
		// The sub-worker reached the provider and the provider refused. That IS
		// deterministic — a malformed prompt or an unknown tool will fail the
		// same way on redelivery.
		return nil, fmt.Errorf("node %s: %s", node.ID, resp.Error)
	}
	return resp.Writes, nil
}

// runIDContextKey carries the current run id into the executor so a delegated
// call can name the run it belongs to, without widening the NodeExecutor
// signature that every other executor implements.
type runIDContextKey struct{}

func withRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDContextKey{}, runID)
}

func runIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(runIDContextKey{}).(string); ok {
		return v
	}
	return ""
}

// ---------------------------------------------------------------------------
// Providers — what a sub-worker actually calls.
// ---------------------------------------------------------------------------

// LLMProvider generates a completion. Kept deliberately small: the sub-worker
// owns retries, timeouts and event emission, so a provider only has to turn a
// prompt into text.
type LLMProvider interface {
	Complete(ctx context.Context, model, prompt string, cfg map[string]any) (string, error)
}

// ToolProvider executes a named tool.
type ToolProvider interface {
	Execute(ctx context.Context, name string, args map[string]any) (any, error)
}

// EchoLLM is a deterministic LLMProvider for tests and for a deployment that
// wants the delegation path exercised without a paid API key. It is honest
// about being a stub — it never pretends to be a model.
type EchoLLM struct{}

func (EchoLLM) Complete(_ context.Context, model, prompt string, _ map[string]any) (string, error) {
	return "echo(" + model + "): " + prompt, nil
}

// promptFrom builds the prompt for an llm node: an explicit config.prompt wins,
// otherwise the graph's channel values are rendered as the context. Falling
// back to the state rather than erroring means an llm node with no prompt still
// does something observable instead of failing the run.
func promptFrom(cfg map[string]any, channels map[string]any) string {
	if p, ok := cfg["prompt"].(string); ok && p != "" {
		return p
	}
	b, err := json.Marshal(channels)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// modelFrom reads the model name, defaulting to a clearly-named placeholder so
// a missing model shows up in the output rather than as an empty string.
func modelFrom(cfg map[string]any) string {
	if m, ok := cfg["model"].(string); ok && m != "" {
		return m
	}
	return "unspecified-model"
}

// outputKeyFrom is the channel an llm node writes its completion to.
func outputKeyFrom(cfg map[string]any) string {
	if k, ok := cfg["output_key"].(string); ok && k != "" {
		return k
	}
	return "completion"
}

// mustJSONRaw marshals a value for an observability payload. A value that will
// not marshal becomes null rather than failing: narration must never break the
// node it describes.
func mustJSONRaw(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
