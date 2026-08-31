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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
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
	InterruptBefore any             `json:"interrupt_before,omitempty"`
	InterruptAfter  any             `json:"interrupt_after,omitempty"`
	Checkpoint      json.RawMessage `json:"checkpoint_id,omitempty"`
	Command         json.RawMessage `json:"command,omitempty"`
}

// normalizeCommand validates RunCreate.command — a LangGraph Command supplied
// at CREATE time rather than at resume. The schema is `anyOf: [Command, null]`,
// so the only structural requirement is that it be a JSON object; its fields
// (update / resume / goto) are the worker's contract and are decoded there
// (worker/command.go), which is also where an unknown goto target is caught
// against the actual graph.
//
// Validating shape here rather than semantics keeps the split honest: the
// server knows the wire schema, only the worker knows the graph.
func normalizeCommand(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: command: want an object, got %T", errInterruptSpecInvalid, v)
	}
	if len(obj) == 0 {
		// An empty command is inert; store nothing rather than an empty object
		// so the run looks identical to one created without a command.
		return nil, nil
	}
	// Unknown fields are deliberately ACCEPTED and stored, not rejected. The
	// Command schema does not set additionalProperties: false, and in OpenAPI an
	// absent additionalProperties means extra properties are permitted — so
	// 422ing them would be stricter than the contract and would break a client
	// written against a later Command with a field we do not know yet. They are
	// inert: the worker decodes only update/resume/goto (worker/command.go).
	//
	// The cost is that a typo such as {"resmue": ...} is silently ineffective.
	// That is the contract's tradeoff to make, not this handler's — contrast
	// interrupt_before, where rejection IS schema-mandated because its string
	// branch is an explicit single-value enum.
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("%w: command: %s", errInterruptSpecInvalid, err)
	}
	return b, nil
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

// normalizeCheckpoint validates RunCreate.checkpoint — the checkpoint a new run
// resumes from, which was written by a PREVIOUS run on the same thread
// (snapshots.aggregate_id is a run id, so a checkpoint belongs to a run, and the
// thread is reached by joining through runs).
//
// Two of CheckpointConfig's four fields are REJECTED rather than ignored:
//
//   - checkpoint_ns is LangGraph's subgraph namespacing, and this engine has no
//     subgraph concept at all — GraphDefinition does not nest, and sub-worker
//     delegation is still TARGET in graph-engine.d2 §8. Storing a namespace we
//     cannot honor would be the silent-drop bug this whole line of work exists
//     to remove; adding a column for it would bake in a field with no defined
//     semantics. An explicit 422 says what is true: not supported yet.
//   - checkpoint_map ("checkpoint-specific data") likewise has no defined
//     meaning here.
//
// checkpoint_id is REQUIRED when a checkpoint config is supplied. The schema
// marks it optional, but the field means "the checkpoint to resume from" — with
// no id there is no checkpoint, and quietly treating the config as absent would
// again be a silent drop. This is a judgment call in favour of an explicit
// error over an inert request.
//
// thread_id, if supplied, must agree with the run's own thread; a contradiction
// is rejected rather than silently resolved in favour of one of them.
func normalizeCheckpoint(cfg *CheckpointConfig, threadID uuid.UUID) (json.RawMessage, error) {
	if cfg == nil {
		return nil, nil
	}
	if cfg.CheckpointNs != nil && *cfg.CheckpointNs != "" {
		return nil, fmt.Errorf("%w: checkpoint.checkpoint_ns is not supported: this engine has no subgraphs", errInterruptSpecInvalid)
	}
	if cfg.CheckpointMap != nil && len(*cfg.CheckpointMap) > 0 {
		return nil, fmt.Errorf("%w: checkpoint.checkpoint_map is not supported", errInterruptSpecInvalid)
	}
	if cfg.ThreadId != nil && *cfg.ThreadId != "" && *cfg.ThreadId != threadID.String() {
		return nil, fmt.Errorf("%w: checkpoint.thread_id %q does not match the run's thread %s", errInterruptSpecInvalid, *cfg.ThreadId, threadID)
	}
	if cfg.CheckpointId == nil || *cfg.CheckpointId == "" {
		return nil, fmt.Errorf("%w: checkpoint.checkpoint_id is required to resume from a checkpoint", errInterruptSpecInvalid)
	}
	id, err := parseCheckpointID(*cfg.CheckpointId)
	if err != nil {
		return nil, fmt.Errorf("%w: checkpoint.checkpoint_id must be a checkpoint identifier", errInterruptSpecInvalid)
	}
	return json.RawMessage(strconv.FormatInt(id, 10)), nil
}

// buildRunKwargs validates the run-level LangGraph fields and renders the
// runs.kwargs jsonb. All absent yields "{}" — the column default — so a run
// that sets none is indistinguishable from one created before these fields
// were honored.
func buildRunKwargs(interruptBefore, interruptAfter, command any, checkpoint json.RawMessage) ([]byte, error) {
	before, err := normalizeInterruptSpec("interrupt_before", interruptBefore)
	if err != nil {
		return nil, err
	}
	after, err := normalizeInterruptSpec("interrupt_after", interruptAfter)
	if err != nil {
		return nil, err
	}
	cmd, err := normalizeCommand(command)
	if err != nil {
		return nil, err
	}
	if before == nil && after == nil && cmd == nil && len(checkpoint) == 0 {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(runKwargs{InterruptBefore: before, InterruptAfter: after, Command: cmd, Checkpoint: checkpoint})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errInterruptSpecInvalid, err)
	}
	return b, nil
}

// errCheckpointNotFound marks a checkpoint that does not exist, or exists but
// belongs to another thread's run — the two collapse deliberately, so probing
// ids cannot distinguish "no such checkpoint" from "not yours".
var errCheckpointNotFound = errors.New("checkpoint not found")

// verifyCheckpointOwned resolves the checkpoint against the run's thread BEFORE
// the run is written, so an unusable reference fails as a clean 404 instead of
// producing a queued run that its worker can only fail. Scoping mirrors
// WorkersReadCheckpoint and threads_state.go: a snapshot is reachable only
// through the thread's own runs.
func (s *Server) verifyCheckpointOwned(ctx context.Context, checkpoint json.RawMessage, threadID uuid.UUID) error {
	if len(checkpoint) == 0 {
		return nil
	}
	id, err := strconv.ParseInt(string(checkpoint), 10, 64)
	if err != nil {
		return errCheckpointNotFound
	}
	var exists bool
	if err := s.Tenant.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM snapshots
			WHERE id = $1 AND aggregate_id IN (SELECT id FROM runs WHERE thread_id = $2))`,
		id, threadID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCheckpointNotFound
	}
	return nil
}

// checkpointHTTPError maps a verifyCheckpointOwned failure to 404.
func checkpointHTTPError(err error) error {
	if errors.Is(err, errCheckpointNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "no such checkpoint for this thread")
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
