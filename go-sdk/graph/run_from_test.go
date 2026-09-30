package graph

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunFromCallbacksAndResume(t *testing.T) {
	g := New[map[string]int]("walk")
	g.AddNode("first", NodeFunc[map[string]int](func(_ context.Context, s map[string]int) (map[string]int, error) { s["first"]++; return s, nil }))
	g.AddNode("second", NodeFunc[map[string]int](func(_ context.Context, s map[string]int) (map[string]int, error) { s["second"]++; return s, nil }))
	g.AddEdge("first", "second").SetEntrypoint("first")
	for _, tt := range []struct {
		name, start string
		want        []string
	}{
		{"fresh", "first", []string{"before:first", "after:first:second", "before:second", "after:second:"}},
		{"resume", "second", []string{"before:second", "after:second:"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			state, err := g.RunFrom(context.Background(), map[string]int{}, tt.start,
				func(node, typ string) error { calls = append(calls, "before:"+node); return nil },
				func(node, typ string, _ map[string]int, next string) error {
					calls = append(calls, "after:"+node+":"+next)
					return nil
				})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, tt.want) {
				t.Errorf("callbacks = %v, want %v", calls, tt.want)
			}
			if tt.start == "second" && state["first"] != 0 {
				t.Error("resume reran first node")
			}
		})
	}
}

func TestRunFromCallbackFailureStopsWalk(t *testing.T) {
	g := New[int]("walk")
	g.AddNode("a", NodeFunc[int](func(_ context.Context, s int) (int, error) { return s + 1, nil }))
	g.AddNode("b", NodeFunc[int](func(_ context.Context, s int) (int, error) { return s + 1, nil }))
	g.AddEdge("a", "b").SetEntrypoint("a")
	want := errors.New("checkpoint failed")
	state, err := g.RunFrom(context.Background(), 0, "a", nil, func(_, _ string, _ int, _ string) error { return want })
	if !errors.Is(err, want) || state != 1 {
		t.Fatalf("state=%d, err=%v; want state=1 and checkpoint error", state, err)
	}
}
