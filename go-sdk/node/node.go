// Package node provides reusable node implementations for common workflow patterns.
//
// These nodes can be used directly with [graph.Graph.AddNode] for common tasks
// like LLM completion, tool execution, and conditional routing.
//
// # LLM Node
//
//	llmNode := node.NewLLMNode[*MyState](provider, node.LLMConfig{
//	    Model:       "gpt-4o-mini",
//	    Temperature: 0.7,
//	    GetMessages: func(s *MyState) []llm.Message { return s.Messages },
//	    SetResponse: func(s *MyState, resp string) { s.Response = resp },
//	})
//	g.AddNodeWithType("think", llmNode, "llm")
//
// # Tool Node
//
//	toolNode := node.NewToolNode[*MyState](node.ToolConfig[*MyState]{
//	    Tools: map[string]node.ToolFunc[*MyState]{
//	        "search": func(ctx context.Context, s *MyState, args map[string]any) (*MyState, error) {
//	            s.SearchResults = doSearch(args["query"].(string))
//	            return s, nil
//	        },
//	    },
//	    GetToolCalls: func(s *MyState) []llm.ToolCall { return s.ToolCalls },
//	})
//	g.AddNodeWithType("tools", toolNode, "tool")
package node

import (
	"context"
	"fmt"

	"github.com/duragraph/duragraph-go/llm"
)

// LLMConfig configures an LLM node.
type LLMConfig[S any] struct {
	Model        string
	Temperature  float64
	MaxTokens    int
	SystemPrompt string

	GetMessages  func(S) []llm.Message
	SetResponse  func(S, string)
	SetToolCalls func(S, []llm.ToolCall)
}

// LLMNode is a graph node that calls an LLM provider.
type LLMNode[S any] struct {
	provider llm.Provider
	config   LLMConfig[S]
}

// NewLLMNode creates a new LLM node with the given provider and configuration.
func NewLLMNode[S any](provider llm.Provider, config LLMConfig[S]) *LLMNode[S] {
	return &LLMNode[S]{provider: provider, config: config}
}

// Execute calls the LLM and updates the state with the response.
func (n *LLMNode[S]) Execute(ctx context.Context, state S) (S, error) {
	if n.config.GetMessages == nil {
		return state, fmt.Errorf("LLMNode: GetMessages not configured")
	}

	messages := n.config.GetMessages(state)

	if n.config.SystemPrompt != "" {
		messages = append([]llm.Message{{Role: "system", Content: n.config.SystemPrompt}}, messages...)
	}

	var opts []llm.Option
	if n.config.Model != "" {
		opts = append(opts, llm.WithModel(n.config.Model))
	}
	if n.config.Temperature > 0 {
		opts = append(opts, llm.WithTemperature(n.config.Temperature))
	}
	if n.config.MaxTokens > 0 {
		opts = append(opts, llm.WithMaxTokens(n.config.MaxTokens))
	}

	resp, err := n.provider.Complete(ctx, messages, opts...)
	if err != nil {
		return state, fmt.Errorf("LLMNode: completion failed: %w", err)
	}

	if n.config.SetResponse != nil && resp.Content != "" {
		n.config.SetResponse(state, resp.Content)
	}
	if n.config.SetToolCalls != nil && len(resp.ToolCalls) > 0 {
		n.config.SetToolCalls(state, resp.ToolCalls)
	}

	return state, nil
}

// ToolFunc is a function that executes a tool call and updates state.
type ToolFunc[S any] func(ctx context.Context, state S, args map[string]any) (S, error)

// ToolConfig configures a tool node.
type ToolConfig[S any] struct {
	Tools        map[string]ToolFunc[S]
	GetToolCalls func(S) []llm.ToolCall
}

// ToolNode executes tool calls from the state.
type ToolNode[S any] struct {
	config ToolConfig[S]
}

// NewToolNode creates a new tool execution node.
func NewToolNode[S any](config ToolConfig[S]) *ToolNode[S] {
	return &ToolNode[S]{config: config}
}

// Execute runs all pending tool calls and updates the state.
func (n *ToolNode[S]) Execute(ctx context.Context, state S) (S, error) {
	if n.config.GetToolCalls == nil {
		return state, nil
	}

	calls := n.config.GetToolCalls(state)
	for _, call := range calls {
		tool, ok := n.config.Tools[call.Name]
		if !ok {
			return state, fmt.Errorf("ToolNode: unknown tool %q", call.Name)
		}
		var err error
		state, err = tool(ctx, state, call.Arguments)
		if err != nil {
			return state, fmt.Errorf("ToolNode: tool %q failed: %w", call.Name, err)
		}
	}

	return state, nil
}

// RouterFunc is a node that routes to different next nodes based on state.
// Implement this as a graph.NodeFunc and use graph.AddConditionalEdge instead
// for simpler cases. This is useful when routing logic needs to modify state.
type RouterFunc[S any] struct {
	fn    func(ctx context.Context, state S) (S, error)
	route func(ctx context.Context, state S) (string, error)
}

// NewRouterNode creates a node that processes state and routes conditionally.
func NewRouterNode[S any](
	process func(ctx context.Context, state S) (S, error),
	route func(ctx context.Context, state S) (string, error),
) *RouterFunc[S] {
	return &RouterFunc[S]{fn: process, route: route}
}

// Execute runs the processing function.
func (r *RouterFunc[S]) Execute(ctx context.Context, state S) (S, error) {
	if r.fn != nil {
		return r.fn(ctx, state)
	}
	return state, nil
}

// Route determines the next node.
func (r *RouterFunc[S]) Route(ctx context.Context, state S) (string, error) {
	return r.route(ctx, state)
}
