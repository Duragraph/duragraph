// The 2-step counter graph — the thin executor that proves durable execution.
// Node A sets count=1, node B sets count=2. A checkpoint is written after each
// node so a crash between them resumes at B (not A). Deliberately trivial; the
// real graph engine is a later cycle.
package worker

type CounterExecutor struct{}

// Nodes returns the ordered node ids.
func (CounterExecutor) Nodes() []string { return []string{"A", "B"} }

// Run applies node[step] to state and returns the new state. Node A → count=1,
// node B → count=2 (idempotent given step).
func (CounterExecutor) Run(step int, state map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range state {
		out[k] = v
	}
	out["count"] = step + 1
	return out
}
