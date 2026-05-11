package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// WorkerGraphSource is the narrow port the query handler uses to
// resolve a graph by its registered id when the assistant's own
// graph aggregate is empty. The worker domain implementation lives
// in postgres.WorkerRepository.FindGraphDefinition — we only need
// the lookup, not the rest of the worker port.
//
// Decoupling this here keeps the query handler from depending on
// the full worker.Repository surface (which carries
// register/heartbeat/poll concerns this query has no business with).
type WorkerGraphSource interface {
	FindGraphDefinition(ctx context.Context, graphID string) (*worker.GraphDefinition, error)
}

// GetAssistantGraphHandler handles the get assistant graph query.
//
// Resolution strategy (top-down):
//
//  1. Look up the assistant. 404 if missing.
//  2. Query the workflow.GraphRepository for graphs scoped to the
//     assistant. Nothing writes to this store today, but the
//     `/builder` save-to-backend flow (roadmap v0.8) will. When it
//     ships, an in-app authored graph supersedes the registered one
//     and this branch wins.
//  3. Fall back to the worker-registered graph definition via the
//     assistant's `graph_id`. Python / Go workers register graphs at
//     startup; that's where the actual node + edge topology lives
//     today.
//  4. If both stores are empty (no in-app graph + no worker has
//     registered this graph_id yet), return an empty GraphResult so
//     callers can render "no graph yet" rather than a 404.
type GetAssistantGraphHandler struct {
	assistantRepo workflow.AssistantRepository
	graphRepo     workflow.GraphRepository
	// workerGraphs is optional. If nil the handler skips step 3 of
	// the resolution and returns whatever the workflow.GraphRepository
	// has. Tests that don't exercise the worker fallback can leave
	// this unset.
	workerGraphs WorkerGraphSource
}

// NewGetAssistantGraphHandler creates a new get assistant graph handler.
// workerGraphs may be nil for tests; production wiring should pass a
// real WorkerGraphSource so the assistant's registered graph
// definition is returned when no in-app graph has been authored yet.
func NewGetAssistantGraphHandler(
	assistantRepo workflow.AssistantRepository,
	graphRepo workflow.GraphRepository,
	workerGraphs WorkerGraphSource,
) *GetAssistantGraphHandler {
	return &GetAssistantGraphHandler{
		assistantRepo: assistantRepo,
		graphRepo:     graphRepo,
		workerGraphs:  workerGraphs,
	}
}

// GraphResult represents the result of a graph query.
type GraphResult struct {
	Nodes  []workflow.Node
	Edges  []workflow.Edge
	Config map[string]interface{}
}

// Handle executes the get assistant graph query (see resolution
// strategy in the handler doc comment).
func (h *GetAssistantGraphHandler) Handle(ctx context.Context, assistantID string) (*GraphResult, error) {
	assistant, err := h.assistantRepo.FindByID(ctx, assistantID)
	if err != nil {
		return nil, errors.NotFound("assistant", assistantID)
	}

	// 1) In-app authored graph (workflow.GraphRepository). Currently
	// always empty — the /builder save-path lands in v0.8.
	graphs, err := h.graphRepo.FindByAssistantID(ctx, assistantID)
	if err != nil {
		return nil, err
	}
	if len(graphs) > 0 {
		graph := graphs[0] // ordered created_at DESC, take latest
		return &GraphResult{
			Nodes:  graph.Nodes(),
			Edges:  graph.Edges(),
			Config: graph.Config(),
		}, nil
	}

	// 2) Worker-registered graph definition, looked up by the
	// assistant's graph_id. This is the only path that produces a
	// non-empty graph today (until the builder backend lands).
	if h.workerGraphs != nil && assistant.GraphID() != "" {
		def, lookupErr := h.workerGraphs.FindGraphDefinition(ctx, assistant.GraphID())
		if lookupErr == nil && def != nil {
			return workerGraphToResult(def), nil
		}
	}

	// 3) Empty fallback. Callers render an empty state rather than a
	// 404 — an assistant without a registered graph is still a valid
	// resource (admin may be in the middle of provisioning).
	return &GraphResult{
		Nodes:  []workflow.Node{},
		Edges:  []workflow.Edge{},
		Config: map[string]interface{}{},
	}, nil
}

// workerGraphToResult maps the worker-side GraphDefinition (no
// `position` on nodes, no edge IDs, string-typed condition) into the
// workflow-side GraphResult that the HTTP DTO expects.
//
//   - Position: stays nil. The dashboard's GraphVisualizer auto-
//     layouts when positions are missing, so we don't need to
//     synthesise placeholders here. When the builder lands and
//     persists positions, this branch goes away.
//   - Edge ID: worker-side edges don't carry an ID — synthesise a
//     stable `source->target` one so xyflow can key React lists.
//     The frontend has a `${source}->${target}#${i}` fallback for
//     the same reason; doing it here keeps the wire payload usable
//     to other clients (Go SDK, conformance tests).
//   - Edge condition: worker model is a bare string ("approved" /
//     "denied"); the workflow.Edge.Condition is map[string]any.
//     Stash the string under {"label": "..."} so the dashboard's
//     edge labelling can pull a stable key when it's wired up. v0.8
//     should converge the two domains' edge schema.
func workerGraphToResult(def *worker.GraphDefinition) *GraphResult {
	nodes := make([]workflow.Node, len(def.Nodes))
	for i, n := range def.Nodes {
		nodes[i] = workflow.Node{
			ID:     n.ID,
			Type:   workflow.NodeType(n.Type),
			Config: n.Config,
		}
	}
	edges := make([]workflow.Edge, len(def.Edges))
	for i, e := range def.Edges {
		var cond map[string]interface{}
		if e.Condition != "" {
			cond = map[string]interface{}{"label": e.Condition}
		}
		edges[i] = workflow.Edge{
			ID:        e.Source + "->" + e.Target,
			Source:    e.Source,
			Target:    e.Target,
			Condition: cond,
		}
	}
	return &GraphResult{
		Nodes:  nodes,
		Edges:  edges,
		Config: map[string]interface{}{},
	}
}
