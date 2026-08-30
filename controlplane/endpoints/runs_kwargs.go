// Run-level LangGraph kwargs — the RunCreate fields that configure HOW a run
// executes rather than what it contains. They are persisted to runs.kwargs,
// which postgres.d2:120 declares for exactly this ("jsonb — run kwargs
// (LangGraph)") and which nothing populated before.
//
// This slice carries interrupt_before / interrupt_after, the run-level half of
// HITL. They are a DIFFERENT axis from the graph's own node config: the graph
// author marks a node config.interrupt_before, while a CALLER names nodes to
// interrupt at for this run only. Both are real, and the worker takes their
// union — see worker/graph.go interruptPolicy.
//
// Contract: OpenAPI RunCreateStateful / RunCreateStateless / CronCreate, each
// declaring `anyOf: [string enum "*", array of string]`. Both fields were
// already in the generated types (types_gen.go InterruptBefore/InterruptAfter,
// typed interface{} because of that anyOf) and were referenced by no handler,
// so a caller's interrupt_before was accepted with a 201 and silently dropped.
package endpoints

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// interruptAll is the wildcard form: interrupt at every node in the graph.
const interruptAll = "*"

// errInterruptSpecInvalid marks a malformed interrupt_before/interrupt_after so
// the caller gets 422 (semantically invalid) rather than a 500.
var errInterruptSpecInvalid = errors.New("invalid interrupt spec")

// runKwargs is the persisted shape of runs.kwargs. Fields are omitted when
// absent so a run created without them stores {} rather than a bag of nulls.
type runKwargs struct {
	InterruptBefore any `json:"interrupt_before,omitempty"`
	InterruptAfter  any `json:"interrupt_after,omitempty"`
}

// normalizeInterruptSpec validates one run-level interrupt field and returns
// the value to persist: nil (absent), the "*" wildcard, or a []string of node
// names. Values arrive as interface{} because the OpenAPI anyOf makes the
// generated field untyped, so this is where the schema is actually enforced —
// without it a caller could store arbitrary JSON in kwargs and the worker would
// have to defend against it at execution time.
func normalizeInterruptSpec(field string, v any) (any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		// The schema's string branch is an enum of exactly one value; a bare
		// node name is NOT valid here (a single node must be a one-element
		// list), so accepting it would invent contract.
		if t != interruptAll {
			return nil, fmt.Errorf("%w: %s: the only allowed string is %q; name a single node as a one-element list", errInterruptSpecInvalid, field, interruptAll)
		}
		return interruptAll, nil
	case []any:
		out := make([]string, 0, len(t))
		for i, e := range t {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("%w: %s[%d]: node names must be strings, got %T", errInterruptSpecInvalid, field, i, e)
			}
			if s == "" {
				return nil, fmt.Errorf("%w: %s[%d]: node name must not be empty", errInterruptSpecInvalid, field, i)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%w: %s: want %q or a list of node names, got %T", errInterruptSpecInvalid, field, interruptAll, v)
	}
}

// buildRunKwargs validates the run-level interrupt fields and renders the
// runs.kwargs jsonb. Absent on both sides yields "{}" — the column default —
// so a run that sets neither is indistinguishable from one created before this
// slice existed.
func buildRunKwargs(interruptBefore, interruptAfter any) ([]byte, error) {
	before, err := normalizeInterruptSpec("interrupt_before", interruptBefore)
	if err != nil {
		return nil, err
	}
	after, err := normalizeInterruptSpec("interrupt_after", interruptAfter)
	if err != nil {
		return nil, err
	}
	if before == nil && after == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(runKwargs{InterruptBefore: before, InterruptAfter: after})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errInterruptSpecInvalid, err)
	}
	return b, nil
}

// interruptSpecHTTPError maps a buildRunKwargs failure to 422. The value parsed
// as JSON (Bind succeeded) but does not satisfy the schema, which is
// Unprocessable Entity rather than Bad Request.
func interruptSpecHTTPError(err error) error {
	if errors.Is(err, errInterruptSpecInvalid) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}
