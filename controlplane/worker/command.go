// The LangGraph resume Command — the payload a client POSTs to
// /threads/{tid}/runs/{rid}/resume to un-park a run, carried to the worker as
// GraphCommand.Resume.
//
// Contract: the OpenAPI Command schema (spec/api/duragraph-latest.yaml) is the
// wire shape — update, resume, goto. graph-engine.d2 §5 on_resume additionally
// lists "send", but there is no `send` field in the schema: a Send is one of
// the shapes `goto` may take, so send folds into goto rather than being its own
// key. OpenAPI wins on wire shape (the precedent set when endpoints.yaml's
// stale assistant-version steps lost to the schema).
//
// Semantics, reconciling the two places the spec describes the merge:
//
//	graph-engine.d2 §5 on_resume  "command.resume/update/goto/send → channel_values"
//	hitl.d2 resume_behavior #5    "Inject Command.resume: as input to that node"
//
// Both are satisfied by landing the resume value IN channel_values (which is
// what the interrupted node reads as its input) under the reserved `resume`
// key — the field's own name, the least inventive binding available, since the
// spec names no key for it.
package worker

import (
	"encoding/json"
	"fmt"
)

// resumeChannelKey is where Command.resume lands in channel_values. The
// interrupted node reads it as its input, and edge conditions can route on it.
const resumeChannelKey = "resume"

// sendTarget is the OpenAPI Send schema: a node to navigate to, plus the input
// to run it with. Both fields are required by the schema.
type sendTarget struct {
	Node  string          `json:"node"`
	Input json.RawMessage `json:"input"`
}

// resumeCommand is the decoded Command. Goto stays raw because the schema
// declares it as an anyOf over four shapes (Send, []Send, string, []string)
// that only a custom decode can collapse.
type resumeCommand struct {
	Update map[string]any  `json:"update,omitempty"`
	Resume json.RawMessage `json:"resume,omitempty"`
	Goto   json.RawMessage `json:"goto,omitempty"`
}

// gotoTargets is the normalized form of Command.goto: the nodes to navigate to,
// in order, each with the optional state patch its Send carried.
type gotoTargets struct {
	Nodes []string
	// Patch is the union of every Send.input that decoded to an object,
	// merged into channel_values before the targets run. A Send whose input is
	// a bare scalar or array has no channel name to bind to, so it is carried
	// as the target's `resume` value instead — the same reserved key
	// Command.resume uses, since both mean "the input this node resumes with".
	Patch map[string]any
}

// decodeGoto collapses the four shapes Command.goto may take into an ordered
// node list plus the state patch its Send inputs carry. A null/absent goto
// yields no targets (routing proceeds normally).
//
// The array forms are attempted before the scalar forms because a JSON array
// cannot decode into a string or a Send, so the ordering is unambiguous.
func decodeGoto(raw json.RawMessage) (gotoTargets, error) {
	var out gotoTargets
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}

	// []string — ["a","b"]
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		out.Nodes = names
		return out, out.validate()
	}
	// []Send — [{"node":"a","input":{...}}]
	var sends []sendTarget
	if err := json.Unmarshal(raw, &sends); err == nil {
		for _, s := range sends {
			out.Nodes = append(out.Nodes, s.Node)
			out.absorb(s)
		}
		return out, out.validate()
	}
	// string — "a"
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		out.Nodes = []string{name}
		return out, out.validate()
	}
	// Send — {"node":"a","input":{...}}
	var send sendTarget
	if err := json.Unmarshal(raw, &send); err == nil {
		out.Nodes = []string{send.Node}
		out.absorb(send)
		return out, out.validate()
	}
	return out, fmt.Errorf("worker: command.goto is not a node name, Send, or list of either: %s", raw)
}

// absorb folds one Send's input into the target patch. An object input merges
// key-by-key; any other JSON value binds to the reserved resume key, because a
// bare scalar carries no channel name of its own.
func (g *gotoTargets) absorb(s sendTarget) {
	if len(s.Input) == 0 || string(s.Input) == "null" {
		return
	}
	if g.Patch == nil {
		g.Patch = map[string]any{}
	}
	var obj map[string]any
	if err := json.Unmarshal(s.Input, &obj); err == nil && obj != nil {
		for k, v := range obj {
			g.Patch[k] = v
		}
		return
	}
	var scalar any
	if err := json.Unmarshal(s.Input, &scalar); err == nil {
		g.Patch[resumeChannelKey] = scalar
	}
}

// validate rejects a goto naming an empty node, which would silently redirect
// the walk to nowhere.
func (g gotoTargets) validate() error {
	for _, n := range g.Nodes {
		if n == "" {
			return fmt.Errorf("worker: command.goto names an empty node")
		}
	}
	return nil
}

// apply merges this Command into channels and reports where the walk should go
// next. It is the whole of graph-engine.d2 §5 on_resume's "merge" step:
//
//   - update patches channel_values key-by-key.
//   - resume lands under the reserved resume key (see resumeChannelKey).
//   - goto contributes its Send inputs to the same patch, and returns the
//     targets that redirect the frontier.
//
// Order matters: update lands first so a resume value or Send input for the
// same key wins over it — the more specific signal about THIS pause beats the
// general state patch.
func (c resumeCommand) apply(channels map[string]any) (gotoTargets, error) {
	for k, v := range c.Update {
		channels[k] = v
	}
	if len(c.Resume) > 0 && string(c.Resume) != "null" {
		var v any
		if err := json.Unmarshal(c.Resume, &v); err != nil {
			return gotoTargets{}, fmt.Errorf("worker: decode command.resume: %w", err)
		}
		channels[resumeChannelKey] = v
	}
	targets, err := decodeGoto(c.Goto)
	if err != nil {
		return gotoTargets{}, err
	}
	for k, v := range targets.Patch {
		channels[k] = v
	}
	return targets, nil
}
