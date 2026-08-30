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
// testable offline. All three of hitl.d2's interrupt triggers are implemented
// (interrupt_before, interrupt_after, requires_human — see pausesAfter), as is
// the tool_calls→approval trigger from graph-engine.d2 §3. Real llm/tool
// delegation to worker.llm.invoke / worker.tool.execute remains TARGET, as does
// the end node's output-freeze to runs.output.
package worker

import (
	"context"
	"encoding/json"
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

// Node types that carry engine meaning. nodeTypeHuman exists purely to collect
// human input: graph-engine.d2 §3 human_ex specifies it as "always interrupt —
// requires_human", so it suspends unconditionally regardless of config.
const nodeTypeHuman = "human"

// The interrupts.reason CHECK constants (003_run.up.sql:43) —
// tool_call | approval_required | input_needed. reasonToolCall is recorded when
// a tool node emits pending tool_calls for human approval (graph-engine.d2 §3
// tool_ex "may: emit tool_calls → interrupt (approval)").
const (
	reasonToolCall         = "tool_call"
	reasonApprovalRequired = "approval_required"
	reasonInputNeeded      = "input_needed"
)

// toolCallsKey is the channel-write key a tool node uses to surface pending
// tool calls that need a human decision. Its presence is itself an interrupt
// trigger (graph-engine.d2 §3 tool_ex), and the value is persisted to
// interrupts.tool_calls (hitl.d2 on_interrupt step 2).
const toolCallsKey = "tool_calls"

// requiresHumanKey is the channel-write key a node uses to signal, from its own
// output, that it needs human input — graph-engine.d2 §5 triggers
// "requires_human: node output signals need for input".
const requiresHumanKey = "requires_human"

// interruptsBefore reports whether this node suspends the run for
// human-in-the-loop BEFORE it executes: either config.interrupt_before is set,
// or the node is a "human" node, which graph-engine.d2 §3 defines as always
// interrupting. Pausing BEFORE (rather than after) is what makes a human node
// meaningful — the walk parks, the client supplies the human's contribution via
// the resume Command, and the node then executes with that state in channels.
// The first of hitl.d2's three triggers.
func (n Node) interruptsBefore() bool {
	if n.Type == nodeTypeHuman {
		return true
	}
	v, _ := n.Config["interrupt_before"].(bool)
	return v
}

// interruptsAfter reports whether this node suspends the run AFTER it executes
// and its writes are merged (config.interrupt_after: true). The second of
// hitl.d2's three triggers.
func (n Node) interruptsAfter() bool {
	v, _ := n.Config["interrupt_after"].(bool)
	return v
}

// interruptPolicy is the RUN-LEVEL interrupt spec, from the caller's
// RunCreate.interrupt_before / interrupt_after (persisted to runs.kwargs and
// forwarded on GraphCommand.Kwargs). It is a second, independent axis from the
// per-node config above: the graph author marks nodes in the definition, while
// a caller names nodes for one run only. Neither overrides the other — a node
// pauses if EITHER says so, because both are affirmative statements that a
// human should see this node, and honouring only one would silently discard the
// other party's intent.
//
// Each field is nil (unset), the "*" wildcard, or an explicit node list, per the
// OpenAPI anyOf. Validation happens server-side at create (endpoints
// normalizeInterruptSpec), so anything arriving here has already been checked.
type interruptPolicy struct {
	Before []string
	After  []string
	// AllBefore/AllAfter record the "*" wildcard, which matches every node —
	// distinct from an empty list, which matches none.
	AllBefore bool
	AllAfter  bool
}

// interruptsBefore reports whether the run-level spec names this node.
func (p interruptPolicy) interruptsBefore(nodeID string) bool {
	return p.AllBefore || contains(p.Before, nodeID)
}

// interruptsAfter reports whether the run-level spec names this node.
func (p interruptPolicy) interruptsAfter(nodeID string) bool {
	return p.AllAfter || contains(p.After, nodeID)
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// decodeInterruptPolicy reads the run-kwargs bag into an interruptPolicy. A
// malformed bag is NOT an error: the server validated the spec at create time,
// so anything unreadable here is a corrupt or hand-edited row, and dropping a
// run on it would be worse than executing with no run-level interrupts.
func decodeInterruptPolicy(kwargs json.RawMessage) interruptPolicy {
	var p interruptPolicy
	if len(kwargs) == 0 {
		return p
	}
	var bag struct {
		Before json.RawMessage `json:"interrupt_before,omitempty"`
		After  json.RawMessage `json:"interrupt_after,omitempty"`
	}
	if err := json.Unmarshal(kwargs, &bag); err != nil {
		return p
	}
	p.AllBefore, p.Before = decodeNodeSelector(bag.Before)
	p.AllAfter, p.After = decodeNodeSelector(bag.After)
	return p
}

// decodeNodeSelector collapses one interrupt_before/interrupt_after value into
// (wildcard, node list) — the two shapes the OpenAPI anyOf allows.
func decodeNodeSelector(raw json.RawMessage) (all bool, nodes []string) {
	if len(raw) == 0 {
		return false, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		return false, names
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s == interruptAllNodes {
		return true, nil
	}
	return false, nil
}

// interruptAllNodes is the wildcard form of a run-level interrupt spec.
const interruptAllNodes = "*"

// pausesAfter reports whether the walk must suspend AFTER this node has run,
// and with which interrupts.reason. It folds the two post-execution triggers
// the spec names, in precedence order:
//
//   - pending tool_calls in the node's writes (graph-engine.d2 §3 tool_ex) →
//     reason tool_call. Most specific: there is a concrete decision to approve.
//   - requires_human, either statically via config.requires_human (hitl.d2
//     graph_node.definition) or dynamically when the node's own output carries
//     it (graph-engine.d2 §5 triggers) → reason input_needed.
//   - config.interrupt_after → the node's configured reason.
//
// writes (not the merged channel state) is the input on purpose: a
// requires_human or tool_calls value that arrived in the RUN'S INPUT would
// otherwise pause the walk at every single node.
func (n Node) pausesAfter(writes map[string]any) (bool, string) {
	if len(n.toolCalls(writes)) > 0 {
		return true, reasonToolCall
	}
	if v, _ := n.Config[requiresHumanKey].(bool); v {
		return true, reasonInputNeeded
	}
	if v, _ := writes[requiresHumanKey].(bool); v {
		return true, reasonInputNeeded
	}
	if n.interruptsAfter() {
		return true, n.interruptReason()
	}
	return false, ""
}

// toolCalls returns the pending tool calls this node emitted, if any. Only a
// non-empty list counts: an empty array is a tool node reporting it needs no
// approval, not an interrupt trigger.
func (n Node) toolCalls(writes map[string]any) []any {
	calls, _ := writes[toolCallsKey].([]any)
	return calls
}

// interruptReason returns the HITL interrupt reason recorded when this node
// suspends the run (config.interrupt_reason). Defaults to input_needed for a
// human node (it exists to ask a human for input) and approval_required
// otherwise. The value must be one of the interrupts.reason CHECK constants
// (tool_call | approval_required | input_needed).
func (n Node) interruptReason() string {
	if s, ok := n.Config["interrupt_reason"].(string); ok && s != "" {
		return s
	}
	if n.Type == nodeTypeHuman {
		return reasonInputNeeded
	}
	return reasonApprovalRequired
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

// defaultExecutors maps Node.type → its executor for this slice, covering every
// type graph-engine.d2 §3 names. start/end/conditional/human pass through
// (start's seeding happens before the loop; end's output-freeze to runs.output
// is a server-side TARGET; conditional's routing lives in the edge evaluator;
// human contributes no writes of its own — it exists to suspend, and the state
// it "produces" arrives via the resume Command). llm/tool are stood in by
// configExecutor — declarative, deterministic, dependency-free — until real
// sub-worker delegation lands.
//
// human MUST have an entry even though it does nothing: without one, the walk
// hits the "no executor for node type" guard and fails the run outright on the
// delivery that resumes past the interrupt.
func defaultExecutors() map[string]NodeExecutor {
	return map[string]NodeExecutor{
		"start":       passthroughExecutor{},
		"end":         passthroughExecutor{},
		"conditional": passthroughExecutor{},
		nodeTypeHuman: passthroughExecutor{},
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
