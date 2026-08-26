// The graph interpreter — the runtime half of the engine mapped in
// spec/models/d2/graph-engine.d2. It turns a GraphDefinition (loaded from the
// graphs table) into an edge-driven frontier walk: start at the entry node(s),
// execute each node against the channel_values state, merge its writes, follow
// its edges (evaluating conditions) to extend the frontier, and stop when the
// frontier empties. The Runner (runner.go) drives this walk with the durable
// checkpoint/epoch/ack spine wrapped around each node boundary.
//
// Slice scope (see the d2 legend): node typing + edge routing + input seeding
// are IMPLEMENTED here; the executors are deterministic and self-contained (no
// LLM keys, no sub-worker delegation) so the whole engine is testcontainers-
// testable offline. HITL interrupts and real llm/tool delegation to
// worker.llm.invoke / worker.tool.execute are TARGET — later slices.
package worker

import (
	"context"
	"fmt"
	"strings"
)

// defaultMaxIterations bounds the super-step loop when a graph's config does
// not set max_iterations. A cyclic or fan-out-heavy graph that never drains its
// frontier fails as run.failed rather than spinning forever.
const defaultMaxIterations = 1000

// Node is one vertex of a GraphDefinition, loaded from the graphs table
// (nodes jsonb). Type dispatches to a NodeExecutor; Config holds the node's
// per-type settings. Mirrors spec/models/d2/graph-entity.d2 (Node{id,type,config}).
type Node struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// interruptsBefore reports whether this node suspends the run for
// human-in-the-loop BEFORE it executes (config.interrupt_before: true). Mirrors
// the hitl.d2 interrupt_before trigger. interrupt_after / requires_human are
// later slices.
func (n Node) interruptsBefore() bool {
	v, _ := n.Config["interrupt_before"].(bool)
	return v
}

// interruptReason returns the HITL interrupt reason recorded when this node
// suspends the run (config.interrupt_reason), defaulting to "approval_required".
// The value must be one of the interrupts.reason CHECK constants
// (tool_call | approval_required | input_needed).
func (n Node) interruptReason() string {
	if s, ok := n.Config["interrupt_reason"].(string); ok && s != "" {
		return s
	}
	return "approval_required"
}

// Edge connects Source→Target, optionally guarded by Condition (an expression
// over channel_values; empty = unconditional). Mirrors graph-entity.d2
// (Edge{source,target,condition}).
type Edge struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Condition string `json:"condition,omitempty"`
}

// GraphDefinition is a runnable graph as stored in the graphs table
// (nodes/edges/config jsonb). Config carries engine settings such as
// max_iterations. Mirrors graph-entity.d2 (GraphDefinition{nodes,edges,config}).
type GraphDefinition struct {
	Nodes  []Node         `json:"nodes"`
	Edges  []Edge         `json:"edges"`
	Config map[string]any `json:"config,omitempty"`
}

// node looks up a node by id.
func (g GraphDefinition) node(id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// entryNodes returns the nodes the walk starts from: every node of type
// "start" if any exist, otherwise every zero-in-degree node (no incoming
// edge). Order follows node-definition order so the walk is deterministic.
func (g GraphDefinition) entryNodes() []string {
	var starts []string
	for _, n := range g.Nodes {
		if n.Type == "start" {
			starts = append(starts, n.ID)
		}
	}
	if len(starts) > 0 {
		return starts
	}
	hasIncoming := map[string]bool{}
	for _, e := range g.Edges {
		hasIncoming[e.Target] = true
	}
	var roots []string
	for _, n := range g.Nodes {
		if !hasIncoming[n.ID] {
			roots = append(roots, n.ID)
		}
	}
	return roots
}

// successors returns the target nodes reachable from nodeID given the current
// channel state: unconditional edges always, conditional edges only when their
// condition evaluates true. Order follows edge-definition order. An
// unparseable condition is a deterministic error (surfaces as run.failed,
// never a silent skip).
func (g GraphDefinition) successors(nodeID string, channels map[string]any) ([]string, error) {
	var out []string
	for _, e := range g.Edges {
		if e.Source != nodeID {
			continue
		}
		if e.Condition == "" {
			out = append(out, e.Target)
			continue
		}
		ok, err := evalCondition(e.Condition, channels)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, e.Target)
		}
	}
	return out, nil
}

// maxIterations reads config.max_iterations (a positive JSON number), falling
// back to defaultMaxIterations. Numbers arrive as float64 from JSON decoding.
func (g GraphDefinition) maxIterations() int {
	if g.Config != nil {
		switch n := g.Config["max_iterations"].(type) {
		case float64:
			if n > 0 {
				return int(n)
			}
		case int:
			if n > 0 {
				return n
			}
		}
	}
	return defaultMaxIterations
}

// evalCondition evaluates a minimal edge condition of the form
// "<channel> == <literal>" or "<channel> != <literal>" against channels.
// Comparison is by canonical string form, so `count == 2` matches whether count
// is stored as the number 2 or the string "2". A literal may be bare or quoted.
// Anything richer (&&, <, function routers) is TARGET (graph-engine.d2 §3) and
// returns an error rather than silently failing open.
func evalCondition(expr string, channels map[string]any) (bool, error) {
	// "==" is checked before "!=" only for locating the operator; the two do
	// not overlap ("a != b" contains no "==").
	if i := strings.Index(expr, "=="); i >= 0 {
		lhs := strings.TrimSpace(expr[:i])
		rhs := strings.Trim(strings.TrimSpace(expr[i+2:]), `"'`)
		return canon(channels[lhs]) == rhs, nil
	}
	if i := strings.Index(expr, "!="); i >= 0 {
		lhs := strings.TrimSpace(expr[:i])
		rhs := strings.Trim(strings.TrimSpace(expr[i+2:]), `"'`)
		return canon(channels[lhs]) != rhs, nil
	}
	return false, fmt.Errorf("worker: unsupported edge condition %q", expr)
}

// canon renders a channel value as the canonical string used for condition
// comparison. A missing value is the empty string. float64(2) renders as "2"
// (Go's default %v), so JSON numbers compare cleanly against integer literals.
func canon(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// NodeExecutor runs one node against the current channel state and returns the
// writes to merge back into channel_values (nil = no writes). A non-nil error
// is a DETERMINISTIC (poison) failure: the Runner records run.failed and does
// NOT redeliver the command.
type NodeExecutor interface {
	Execute(ctx context.Context, node Node, channels map[string]any) (writes map[string]any, err error)
}

// defaultExecutors maps Node.type → its executor for this slice. start/end/
// conditional pass through (start's seeding happens before the loop; end's
// output-freeze is a server-side TARGET; conditional's routing lives in the
// edge evaluator). llm/tool are stood in by configExecutor — declarative,
// deterministic, dependency-free — until real sub-worker delegation lands.
func defaultExecutors() map[string]NodeExecutor {
	return map[string]NodeExecutor{
		"start":       passthroughExecutor{},
		"end":         passthroughExecutor{},
		"conditional": passthroughExecutor{},
		"llm":         configExecutor{},
		"tool":        configExecutor{},
	}
}

// passthroughExecutor writes nothing and never fails.
type passthroughExecutor struct{}

func (passthroughExecutor) Execute(context.Context, Node, map[string]any) (map[string]any, error) {
	return nil, nil
}

// configExecutor is the deterministic slice-1 stand-in for llm/tool nodes: it
// applies node.config declaratively instead of calling a provider or a
// sub-worker. `set` (an object) merges into channel_values; `fail` (a string)
// makes the node deterministically poison. This keeps the engine end-to-end
// testable with no external dependencies; real delegation to worker.llm.invoke
// / worker.tool.execute is TARGET (graph-engine.d2 §8).
type configExecutor struct{}

func (configExecutor) Execute(_ context.Context, node Node, _ map[string]any) (map[string]any, error) {
	if msg, ok := node.Config["fail"].(string); ok {
		return nil, fmt.Errorf("node %s: %s", node.ID, msg)
	}
	if set, ok := node.Config["set"].(map[string]any); ok {
		return set, nil
	}
	return nil, nil
}
