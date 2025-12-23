package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// GetSubgraphsHandler handles the get subgraphs query
type GetSubgraphsHandler struct {
	assistantRepo workflow.AssistantRepository
	graphRepo     workflow.GraphRepository
}

// NewGetSubgraphsHandler creates a new get subgraphs handler
func NewGetSubgraphsHandler(
	assistantRepo workflow.AssistantRepository,
	graphRepo workflow.GraphRepository,
) *GetSubgraphsHandler {
	return &GetSubgraphsHandler{
		assistantRepo: assistantRepo,
		graphRepo:     graphRepo,
	}
}

// SubgraphInfo represents information about a subgraph
type SubgraphInfo struct {
	Namespace string `json:"namespace"`
	GraphID   string `json:"graph_id"`
}

// Handle executes the get subgraphs query
func (h *GetSubgraphsHandler) Handle(ctx context.Context, assistantID string) ([]SubgraphInfo, error) {
	// Verify assistant exists
	_, err := h.assistantRepo.FindByID(ctx, assistantID)
	if err != nil {
		return nil, errors.NotFound("assistant", assistantID)
	}

	// Get graphs for this assistant
	graphs, err := h.graphRepo.FindByAssistantID(ctx, assistantID)
	if err != nil {
		return nil, err
	}

	// If no graphs exist, return empty list
	if len(graphs) == 0 {
		return []SubgraphInfo{}, nil
	}

	// Find subgraph nodes in the latest graph
	graph := graphs[0]
	subgraphs := []SubgraphInfo{}

	for _, node := range graph.Nodes() {
		if node.Type == workflow.NodeTypeSubgraph {
			// Extract namespace and graph_id from config
			namespace := node.ID
			graphID := ""

			if ns, ok := node.Config["namespace"].(string); ok {
				namespace = ns
			}
			if gid, ok := node.Config["graph_id"].(string); ok {
				graphID = gid
			}

			subgraphs = append(subgraphs, SubgraphInfo{
				Namespace: namespace,
				GraphID:   graphID,
			})
		}
	}

	return subgraphs, nil
}

// HandleByNamespace retrieves a specific subgraph by namespace
func (h *GetSubgraphsHandler) HandleByNamespace(ctx context.Context, assistantID, namespace string) (*GraphResult, error) {
	// Verify assistant exists
	_, err := h.assistantRepo.FindByID(ctx, assistantID)
	if err != nil {
		return nil, errors.NotFound("assistant", assistantID)
	}

	// Get graphs for this assistant
	graphs, err := h.graphRepo.FindByAssistantID(ctx, assistantID)
	if err != nil {
		return nil, err
	}

	if len(graphs) == 0 {
		return nil, errors.NotFound("subgraph", namespace)
	}

	// Find the subgraph node with matching namespace
	graph := graphs[0]
	for _, node := range graph.Nodes() {
		if node.Type == workflow.NodeTypeSubgraph {
			nodeNamespace := node.ID
			if ns, ok := node.Config["namespace"].(string); ok {
				nodeNamespace = ns
			}

			if nodeNamespace == namespace {
				// Extract the subgraph definition from config
				subNodes := []workflow.Node{}
				subEdges := []workflow.Edge{}
				subConfig := map[string]interface{}{}

				if nodes, ok := node.Config["nodes"].([]interface{}); ok {
					for _, n := range nodes {
						if nodeMap, ok := n.(map[string]interface{}); ok {
							subNodes = append(subNodes, workflow.Node{
								ID:     nodeMap["id"].(string),
								Type:   workflow.NodeType(nodeMap["type"].(string)),
								Config: nodeMap,
							})
						}
					}
				}

				if edges, ok := node.Config["edges"].([]interface{}); ok {
					for _, e := range edges {
						if edgeMap, ok := e.(map[string]interface{}); ok {
							subEdges = append(subEdges, workflow.Edge{
								ID:     edgeMap["id"].(string),
								Source: edgeMap["source"].(string),
								Target: edgeMap["target"].(string),
							})
						}
					}
				}

				if cfg, ok := node.Config["config"].(map[string]interface{}); ok {
					subConfig = cfg
				}

				return &GraphResult{
					Nodes:  subNodes,
					Edges:  subEdges,
					Config: subConfig,
				}, nil
			}
		}
	}

	return nil, errors.NotFound("subgraph", namespace)
}
