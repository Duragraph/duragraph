// LLM and Tool sub-workers — the consuming half of graph-engine.d2 §8.
//
// nats.d2 declares both of them as durable consumers on WORKER_COMMANDS:
//
//	llm-worker  | filter=worker.llm.invoke   | ack_wait=2m
//	tool-worker | filter=worker.tool.execute | ack_wait=1m
//
// They existed as declarations with nothing bound to them. A graph worker could
// publish an invocation and no process would ever answer.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// SubWorker answers invocation requests on one subject.
//
// Replies go over core NATS to the request's reply inbox rather than back onto
// a stream: the graph worker is blocked waiting for this specific answer, so
// the reply has exactly one interested party and no value once that party is
// gone. Persisting it would leave results for runs that had already been
// redelivered and replayed.
type SubWorker struct {
	nc      *nats.Conn
	subject string
	handle  func(context.Context, InvokeRequest) InvokeResponse
	sub     *nats.Subscription
	stopCh  chan struct{}
}

// NewLLMSubWorker binds an LLMProvider to worker.llm.invoke.
func NewLLMSubWorker(nc *nats.Conn, p LLMProvider) *SubWorker {
	return &SubWorker{
		nc:      nc,
		subject: SubjectLLMInvoke,
		stopCh:  make(chan struct{}),
		handle: func(ctx context.Context, req InvokeRequest) InvokeResponse {
			model := modelFrom(req.Config)
			text, err := p.Complete(ctx, model, promptFrom(req.Config, req.Channels), req.Config)
			if err != nil {
				return InvokeResponse{Error: err.Error()}
			}
			// The completion lands on a channel so the rest of the graph can
			// route on it, which is the entire point of running an llm node
			// inside a graph rather than calling the provider directly.
			return InvokeResponse{Writes: map[string]any{outputKeyFrom(req.Config): text}}
		},
	}
}

// NewToolSubWorker binds a ToolProvider to worker.tool.execute.
func NewToolSubWorker(nc *nats.Conn, p ToolProvider) *SubWorker {
	return &SubWorker{
		nc:      nc,
		subject: SubjectToolExecute,
		stopCh:  make(chan struct{}),
		handle: func(ctx context.Context, req InvokeRequest) InvokeResponse {
			name, _ := req.Config["tool"].(string)
			if name == "" {
				// Naming the node makes this actionable; "tool is required" on
				// its own does not say which node is wrong.
				return InvokeResponse{Error: fmt.Sprintf("node %s: config.tool is required", req.NodeID)}
			}
			args, _ := req.Config["args"].(map[string]any)
			if args == nil {
				args = map[string]any{}
			}
			result, err := p.Execute(ctx, name, args)
			if err != nil {
				return InvokeResponse{Error: err.Error()}
			}
			return InvokeResponse{Writes: map[string]any{outputKeyFrom(req.Config): result}}
		},
	}
}

// Start subscribes and serves until ctx is canceled or Stop is called.
func (w *SubWorker) Start(ctx context.Context) error {
	sub, err := w.nc.Subscribe(w.subject, func(msg *nats.Msg) {
		var req InvokeRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			w.reply(msg, InvokeResponse{Error: "malformed invocation: " + err.Error()})
			return
		}
		resp := w.handle(ctx, req)
		if resp.Error != "" {
			slog.Warn("sub-worker: invocation failed",
				"subject", w.subject, "run_id", req.RunID, "node_id", req.NodeID, "err", resp.Error)
		}
		w.reply(msg, resp)
	})
	if err != nil {
		return err
	}
	w.sub = sub

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stopCh:
		return nil
	}
}

// reply answers the request. A caller that has already given up leaves no reply
// subject, which is normal (its run was redelivered) and not worth an error.
func (w *SubWorker) reply(msg *nats.Msg, resp InvokeResponse) {
	if msg.Reply == "" {
		return
	}
	b, err := json.Marshal(resp)
	if err != nil {
		b = []byte(`{"error":"sub-worker produced an unserialisable reply"}`)
	}
	_ = msg.Respond(b)
}

// Stop unsubscribes and releases Start. Idempotent.
func (w *SubWorker) Stop() {
	if w.sub != nil {
		_ = w.sub.Unsubscribe()
	}
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

// natsInvoker is the production Invoker: a core-NATS request/reply.
type natsInvoker struct{ nc *nats.Conn }

// NewNATSInvoker builds the Invoker a graph worker uses to reach sub-workers.
func NewNATSInvoker(nc *nats.Conn) Invoker { return natsInvoker{nc: nc} }

func (n natsInvoker) Invoke(ctx context.Context, subject string, req InvokeRequest, timeout time.Duration) (InvokeResponse, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return InvokeResponse{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	msg, err := n.nc.RequestWithContext(reqCtx, subject, b)
	if err != nil {
		// nats.ErrNoResponders means no sub-worker is running. Saying so
		// explicitly is the difference between an operator restarting a fleet
		// and an operator debugging a graph.
		return InvokeResponse{}, fmt.Errorf("no reply on %s (is a sub-worker running?): %w", subject, err)
	}
	var resp InvokeResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return InvokeResponse{}, fmt.Errorf("malformed reply on %s: %w", subject, err)
	}
	return resp, nil
}
