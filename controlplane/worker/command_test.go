package worker

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestDecodeGotoShapes covers every shape the OpenAPI Command.goto anyOf
// allows: a node name, a list of names, a Send, and a list of Sends — plus the
// null/absent cases that must mean "route normally", not "route nowhere".
func TestDecodeGotoShapes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       string
		wantNodes []string
		wantPatch map[string]any
	}{
		{name: "absent", raw: ``, wantNodes: nil},
		{name: "null", raw: `null`, wantNodes: nil},
		{name: "string", raw: `"b"`, wantNodes: []string{"b"}},
		{name: "string list", raw: `["b","c"]`, wantNodes: []string{"b", "c"}},
		{
			name:      "send with object input",
			raw:       `{"node":"b","input":{"x":1}}`,
			wantNodes: []string{"b"},
			wantPatch: map[string]any{"x": float64(1)},
		},
		{
			name:      "send list merges every input",
			raw:       `[{"node":"b","input":{"x":1}},{"node":"c","input":{"y":2}}]`,
			wantNodes: []string{"b", "c"},
			wantPatch: map[string]any{"x": float64(1), "y": float64(2)},
		},
		{
			// A bare scalar Send input has no channel name to bind to, so it
			// lands on the same reserved key Command.resume uses.
			name:      "send with scalar input binds to resume key",
			raw:       `{"node":"b","input":"yes"}`,
			wantNodes: []string{"b"},
			wantPatch: map[string]any{resumeChannelKey: "yes"},
		},
		{
			name:      "send with null input contributes no patch",
			raw:       `{"node":"b","input":null}`,
			wantNodes: []string{"b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeGoto(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("decodeGoto(%s): %v", tc.raw, err)
			}
			if !reflect.DeepEqual(got.Nodes, tc.wantNodes) {
				t.Errorf("nodes: want %v, got %v", tc.wantNodes, got.Nodes)
			}
			if len(tc.wantPatch) == 0 && len(got.Patch) != 0 {
				t.Errorf("patch: want none, got %v", got.Patch)
			}
			if len(tc.wantPatch) > 0 && !reflect.DeepEqual(got.Patch, tc.wantPatch) {
				t.Errorf("patch: want %v, got %v", tc.wantPatch, got.Patch)
			}
		})
	}
}

// TestDecodeGotoRejectsGarbage pins that an unusable goto is a hard error, not
// a silent "route normally" — a client typo must not quietly become a no-op.
func TestDecodeGotoRejectsGarbage(t *testing.T) {
	for _, raw := range []string{`5`, `true`, `{"input":{"x":1}}`, `["a",""]`} {
		if _, err := decodeGoto(json.RawMessage(raw)); err == nil {
			t.Errorf("decodeGoto(%s): want an error, got nil", raw)
		}
	}
}

// TestCommandApplyPrecedence pins the merge order graph-engine.d2 §5 on_resume
// implies: update is the general state patch, so a resume value or a Send input
// naming the same key — both more specific statements about THIS pause — win
// over it.
func TestCommandApplyPrecedence(t *testing.T) {
	channels := map[string]any{"keep": "me"}
	var cmd resumeCommand
	if err := json.Unmarshal([]byte(`{
		"update": {"a": 1, "resume": "from-update", "shared": "from-update"},
		"resume": "from-resume",
		"goto":   {"node": "n", "input": {"shared": "from-send"}}
	}`), &cmd); err != nil {
		t.Fatal(err)
	}

	targets, err := cmd.apply(channels)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !reflect.DeepEqual(targets.Nodes, []string{"n"}) {
		t.Errorf("goto nodes: want [n], got %v", targets.Nodes)
	}
	if channels["keep"] != "me" {
		t.Error("apply must not disturb untouched channels")
	}
	if channels["a"] != float64(1) {
		t.Errorf("update key: want 1, got %v", channels["a"])
	}
	if channels[resumeChannelKey] != "from-resume" {
		t.Errorf("resume must beat an update writing the same key: got %v", channels[resumeChannelKey])
	}
	if channels["shared"] != "from-send" {
		t.Errorf("Send input must beat an update writing the same key: got %v", channels["shared"])
	}
}

// TestCommandApplyEmpty pins that an empty command is inert: no channel is
// invented and routing is left to the graph.
func TestCommandApplyEmpty(t *testing.T) {
	channels := map[string]any{}
	var cmd resumeCommand
	if err := json.Unmarshal([]byte(`{}`), &cmd); err != nil {
		t.Fatal(err)
	}
	targets, err := cmd.apply(channels)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(targets.Nodes) != 0 {
		t.Errorf("goto nodes: want none, got %v", targets.Nodes)
	}
	if len(channels) != 0 {
		t.Errorf("channels: want untouched, got %v", channels)
	}
}
