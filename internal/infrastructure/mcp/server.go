package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/application/query"
	"github.com/duragraph/duragraph/internal/application/service"
	"github.com/duragraph/duragraph/internal/infrastructure/tools"
	"github.com/google/uuid"
)

const (
	ProtocolVersion = "2024-11-05"
	ServerName      = "duragraph"
	ServerVersion   = "1.0.0"
)

// JSON-RPC 2.0 types
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP protocol types

type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    Capability `json:"capabilities"`
	ClientInfo      Info       `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string           `json:"protocolVersion"`
	Capabilities    ServerCapability `json:"capabilities"`
	ServerInfo      Info             `json:"serverInfo"`
	Instructions    string           `json:"instructions,omitempty"`
}

type Capability struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
}

type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type SamplingCapability struct{}

type ServerCapability struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type ToolsListResult struct {
	Tools      []ToolDef `json:"tools"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ToolCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type ResourceDef struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type ResourcesListResult struct {
	Resources  []ResourceDef `json:"resources"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type ResourceReadParams struct {
	URI string `json:"uri"`
}

type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

type ResourceReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// Session tracks an MCP client session
type Session struct {
	ID          string
	Initialized bool
}

// Server implements the MCP protocol over Streamable HTTP
type Server struct {
	toolRegistry   *tools.Registry
	assistantQuery *query.GetAssistantHandler
	listAssistants *query.ListAssistantsHandler
	getThread      *query.GetThreadHandler
	createRun      *command.CreateRunHandler
	getRun         *query.GetRunHandler
	runService     *service.RunService

	sessions   map[string]*Session
	sessionsMu sync.RWMutex
}

func NewServer(
	toolRegistry *tools.Registry,
	assistantQuery *query.GetAssistantHandler,
	listAssistants *query.ListAssistantsHandler,
	getThread *query.GetThreadHandler,
	createRun *command.CreateRunHandler,
	getRun *query.GetRunHandler,
	runService *service.RunService,
) *Server {
	return &Server{
		toolRegistry:   toolRegistry,
		assistantQuery: assistantQuery,
		listAssistants: listAssistants,
		getThread:      getThread,
		createRun:      createRun,
		getRun:         getRun,
		runService:     runService,
		sessions:       make(map[string]*Session),
	}
}

func (s *Server) HandleRequest(ctx context.Context, sessionID string, req *Request) *Response {
	if req.JSONRPC != "2.0" {
		return errorResponse(req.ID, -32600, "invalid JSON-RPC version")
	}

	// Notifications have no ID and expect no response
	if req.ID == nil {
		s.handleNotification(sessionID, req)
		return nil
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "ping":
		return successResponse(req.ID, map[string]interface{}{})
	case "tools/list":
		return s.handleToolsList(ctx, req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "resources/list":
		return s.handleResourcesList(ctx, req)
	case "resources/read":
		return s.handleResourcesRead(ctx, req)
	default:
		return errorResponse(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) CreateSession() string {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	id := uuid.New().String()
	s.sessions[id] = &Session{ID: id}
	return id
}

func (s *Server) DeleteSession(id string) bool {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	if _, ok := s.sessions[id]; ok {
		delete(s.sessions, id)
		return true
	}
	return false
}

func (s *Server) HasSession(id string) bool {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	_, ok := s.sessions[id]
	return ok
}

func (s *Server) handleNotification(sessionID string, req *Request) {
	switch req.Method {
	case "notifications/initialized":
		s.sessionsMu.Lock()
		if sess, ok := s.sessions[sessionID]; ok {
			sess.Initialized = true
		}
		s.sessionsMu.Unlock()
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	var params InitializeParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errorResponse(req.ID, -32602, "invalid initialize params")
		}
	}

	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapability{
			Tools:     &ToolsCapability{ListChanged: false},
			Resources: &ResourcesCapability{Subscribe: false, ListChanged: false},
		},
		ServerInfo: Info{
			Name:    ServerName,
			Version: ServerVersion,
		},
		Instructions: "DuraGraph AI workflow orchestration platform. Use tools to invoke assistants and manage threads.",
	}

	return successResponse(req.ID, result)
}

func (s *Server) handleToolsList(ctx context.Context, req *Request) *Response {
	mcpTools := []ToolDef{}

	// Expose registered tools from the tool registry
	for _, t := range s.toolRegistry.List() {
		schema := t.Schema()
		if schema == nil {
			schema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
		}
		mcpTools = append(mcpTools, ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: schema,
		})
	}

	// Expose each assistant as an invocable tool
	assistants, err := s.listAssistants.Handle(ctx, 100, 0)
	if err == nil {
		for _, a := range assistants {
			mcpTools = append(mcpTools, ToolDef{
				Name:        fmt.Sprintf("invoke_assistant_%s", a.ID()),
				Description: fmt.Sprintf("Invoke assistant '%s'. Creates a run and returns the result.", a.Name()),
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"thread_id": map[string]interface{}{
							"type":        "string",
							"description": "Thread ID to run against (optional for stateless runs)",
						},
						"input": map[string]interface{}{
							"type":        "object",
							"description": "Input data for the run",
						},
						"config": map[string]interface{}{
							"type":        "object",
							"description": "Runtime configuration overrides",
						},
					},
					"required": []string{},
				},
			})
		}
	}

	return successResponse(req.ID, ToolsListResult{Tools: mcpTools})
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) *Response {
	var params ToolCallParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errorResponse(req.ID, -32602, "invalid tool call params")
		}
	}

	if params.Name == "" {
		return errorResponse(req.ID, -32602, "tool name is required")
	}

	// Check if it's an assistant invocation tool
	if assistantID, ok := parseAssistantToolName(params.Name); ok {
		return s.invokeAssistant(ctx, req.ID, assistantID, params.Arguments)
	}

	// Otherwise try the tool registry
	result, err := s.toolRegistry.Execute(ctx, params.Name, params.Arguments)
	if err != nil {
		return successResponse(req.ID, ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("Error: %s", err.Error())}},
			IsError: true,
		})
	}

	resultJSON, _ := json.Marshal(result)
	return successResponse(req.ID, ToolCallResult{
		Content: []Content{{Type: "text", Text: string(resultJSON)}},
	})
}

func (s *Server) invokeAssistant(ctx context.Context, reqID json.RawMessage, assistantID string, args map[string]interface{}) *Response {
	threadID, _ := args["thread_id"].(string)
	input, _ := args["input"].(map[string]interface{})
	config, _ := args["config"].(map[string]interface{})

	if input == nil {
		input = map[string]interface{}{}
	}

	cmd := command.CreateRun{
		AssistantID: assistantID,
		ThreadID:    threadID,
		Input:       input,
		Config:      config,
	}

	runID, err := s.createRun.Handle(ctx, cmd)
	if err != nil {
		return successResponse(reqID, ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("Failed to create run: %s", err.Error())}},
			IsError: true,
		})
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"run_id":       runID,
		"assistant_id": assistantID,
		"status":       "queued",
		"message":      "Run created successfully. Use run_id to check status.",
	})

	return successResponse(reqID, ToolCallResult{
		Content: []Content{{Type: "text", Text: string(resultJSON)}},
	})
}

func (s *Server) handleResourcesList(ctx context.Context, req *Request) *Response {
	resources := []ResourceDef{
		{
			URI:         "duragraph://assistants",
			Name:        "Assistants",
			Description: "List of all configured assistants",
			MimeType:    "application/json",
		},
		{
			URI:         "duragraph://server/info",
			Name:        "Server Info",
			Description: "DuraGraph server information and capabilities",
			MimeType:    "application/json",
		},
	}

	return successResponse(req.ID, ResourcesListResult{Resources: resources})
}

func (s *Server) handleResourcesRead(ctx context.Context, req *Request) *Response {
	var params ResourceReadParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errorResponse(req.ID, -32602, "invalid resource read params")
		}
	}

	switch params.URI {
	case "duragraph://assistants":
		assistants, err := s.listAssistants.Handle(ctx, 100, 0)
		if err != nil {
			return errorResponse(req.ID, -32603, fmt.Sprintf("failed to list assistants: %s", err.Error()))
		}

		list := make([]map[string]interface{}, 0, len(assistants))
		for _, a := range assistants {
			list = append(list, map[string]interface{}{
				"id":   a.ID(),
				"name": a.Name(),

				"metadata": a.Metadata(),
			})
		}

		data, _ := json.MarshalIndent(list, "", "  ")
		return successResponse(req.ID, ResourceReadResult{
			Contents: []ResourceContent{{
				URI:      params.URI,
				MimeType: "application/json",
				Text:     string(data),
			}},
		})

	case "duragraph://server/info":
		info := map[string]interface{}{
			"name":             ServerName,
			"version":          ServerVersion,
			"protocol_version": ProtocolVersion,
			"capabilities":     []string{"tools", "resources", "assistants", "threads", "runs", "store", "crons"},
		}
		data, _ := json.MarshalIndent(info, "", "  ")
		return successResponse(req.ID, ResourceReadResult{
			Contents: []ResourceContent{{
				URI:      params.URI,
				MimeType: "application/json",
				Text:     string(data),
			}},
		})

	default:
		return errorResponse(req.ID, -32002, fmt.Sprintf("resource not found: %s", params.URI))
	}
}

// parseAssistantToolName extracts assistant ID from tool name "invoke_assistant_<uuid>"
func parseAssistantToolName(name string) (string, bool) {
	const prefix = "invoke_assistant_"
	if len(name) > len(prefix) && name[:len(prefix)] == prefix {
		return name[len(prefix):], true
	}
	return "", false
}

func successResponse(id json.RawMessage, result interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func errorResponse(id json.RawMessage, code int, message string) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
}
