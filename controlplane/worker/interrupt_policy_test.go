package worker

import (
	"encoding/json"
	"testing"
)

// TestDecodeInterruptPolicy covers the run-level interrupt spec as it arrives
// on GraphCommand.Kwargs: the two shapes the OpenAPI anyOf allows (wildcard
// string, node list), the absent cases, and the corrupt-row case that must
// degrade to "no run-level interrupts" rather than dropping the run.
func TestDecodeInterruptPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		kwargs                string
		wantBefore, wantAfter []string
		wantAllB, wantAllA    bool
	}{
		{name: "absent", kwargs: ``},
		{name: "empty bag", kwargs: `{}`},
		{name: "unrelated kwargs ignored", kwargs: `{"webhook":"https://x"}`},
		{
			name:       "before list",
			kwargs:     `{"interrupt_before":["a","b"]}`,
			wantBefore: []string{"a", "b"},
		},
		{
			name:      "after list",
			kwargs:    `{"interrupt_after":["c"]}`,
			wantAfter: []string{"c"},
		},
		{
			name:     "before wildcard",
			kwargs:   `{"interrupt_before":"*"}`,
			wantAllB: true,
		},
		{
			name:     "both wildcards",
			kwargs:   `{"interrupt_before":"*","interrupt_after":"*"}`,
			wantAllB: true, wantAllA: true,
		},
		{
			name:       "mixed shapes",
			kwargs:     `{"interrupt_before":["a"],"interrupt_after":"*"}`,
			wantBefore: []string{"a"}, wantAllA: true,
		},
		// A corrupt or hand-edited row must not take the run down: the server
		// validated this at create time, so anything unreadable here is an
		// anomaly, and executing with no run-level interrupts beats failing.
		{name: "corrupt bag", kwargs: `not json`},
		{name: "wrong inner type", kwargs: `{"interrupt_before":42}`},
		{name: "unknown wildcard string", kwargs: `{"interrupt_before":"all"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := decodeInterruptPolicy(json.RawMessage(tc.kwargs))
			if p.AllBefore != tc.wantAllB {
				t.Errorf("AllBefore: want %v, got %v", tc.wantAllB, p.AllBefore)
			}
			if p.AllAfter != tc.wantAllA {
				t.Errorf("AllAfter: want %v, got %v", tc.wantAllA, p.AllAfter)
			}
			if len(p.Before) != len(tc.wantBefore) {
				t.Fatalf("Before: want %v, got %v", tc.wantBefore, p.Before)
			}
			for i, n := range tc.wantBefore {
				if p.Before[i] != n {
					t.Errorf("Before[%d]: want %q, got %q", i, n, p.Before[i])
				}
			}
			if len(p.After) != len(tc.wantAfter) {
				t.Fatalf("After: want %v, got %v", tc.wantAfter, p.After)
			}
		})
	}
}

// TestInterruptPolicyMatching pins the two lookups, including the distinction
// that makes the wildcard meaningful: an EMPTY list matches nothing, while "*"
// matches everything.
func TestInterruptPolicyMatching(t *testing.T) {
	empty := decodeInterruptPolicy(json.RawMessage(`{"interrupt_before":[]}`))
	if empty.interruptsBefore("anything") {
		t.Error("an empty list must match no node")
	}

	star := decodeInterruptPolicy(json.RawMessage(`{"interrupt_before":"*"}`))
	if !star.interruptsBefore("anything") {
		t.Error(`"*" must match every node`)
	}
	if star.interruptsAfter("anything") {
		t.Error("interrupt_before wildcard must not leak into interrupt_after")
	}

	named := decodeInterruptPolicy(json.RawMessage(`{"interrupt_after":["b"]}`))
	if !named.interruptsAfter("b") {
		t.Error("a named node must match")
	}
	if named.interruptsAfter("a") {
		t.Error("an unnamed node must not match")
	}
	if named.interruptsBefore("b") {
		t.Error("interrupt_after must not leak into interrupt_before")
	}
}

// TestZeroPolicyIsInert pins that a run with no kwargs behaves exactly as it
// did before this field existed — nothing pauses on the run-level axis.
func TestZeroPolicyIsInert(t *testing.T) {
	var p interruptPolicy
	if p.interruptsBefore("a") || p.interruptsAfter("a") {
		t.Error("the zero policy must never trigger an interrupt")
	}
}
