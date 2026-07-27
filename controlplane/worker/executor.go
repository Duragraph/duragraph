// The 2-step counter graph — the thin executor that proves durable execution.
// Node A sets count=1, node B sets count=2. A checkpoint is written after each
// node so a crash between them resumes at B (not A). Deliberately trivial; the
// real graph engine is a later cycle.
package worker

// Executor is a graph the runner can execute step by step. Run returns the new
// state, or a non-nil error for a DETERMINISTIC (poison) failure — the runner
// turns that into run.failed and does NOT redeliver.
type Executor interface {
	Nodes() []string
	Run(step int, state map[string]int) (map[string]int, error)
}

type CounterExecutor struct{}

// Nodes returns the ordered node ids.
func (CounterExecutor) Nodes() []string { return []string{"A", "B"} }

// Run applies node[step] to state and returns the new state. Node A → count=1,
// node B → count=2 (idempotent given step). Never errors.
func (CounterExecutor) Run(step int, state map[string]int) (map[string]int, error) {
	out := map[string]int{}
	for k, v := range state {
		out[k] = v
	}
	out["count"] = step + 1
	return out, nil
}
