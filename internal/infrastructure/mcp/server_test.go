package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParseAssistantToolName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantID string
		wantOK bool
	}{
		{"valid", "invoke_assistant_abc-123", "abc-123", true},
		{"valid uuid", "invoke_assistant_550e8400-e29b-41d4-a716-446655440000", "550e8400-e29b-41d4-a716-446655440000", true},
		{"no prefix", "run_assistant_abc", "", false},
		{"empty", "", "", false},
		{"only prefix", "invoke_assistant_", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := parseAssistantToolName(tt.input)
			if ok != tt.wantOK {
				t.Errorf("parseAssistantToolName(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if id != tt.wantID {
				t.Errorf("parseAssistantToolName(%q) id = %q, want %q", tt.input, id, tt.wantID)
			}
		})
	}
}

func TestSuccessResponse(t *testing.T) {
	id := json.RawMessage(`1`)
	resp := successResponse(id, map[string]string{"key": "value"})

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Error("expected no error")
	}
	if resp.Result == nil {
		t.Error("expected result")
	}
}

func TestErrorResponse(t *testing.T) {
	id := json.RawMessage(`"req-1"`)
	resp := errorResponse(id, -32601, "method not found")

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
	if resp.Result != nil {
		t.Error("expected no result")
	}
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "method not found" {
		t.Errorf("expected message 'method not found', got %s", resp.Error.Message)
	}
}

func TestHandleRequest_InvalidVersion(t *testing.T) {
	s := &Server{sessions: make(map[string]*Session)}
	req := &Request{JSONRPC: "1.0", ID: json.RawMessage(`1`), Method: "ping"}
	resp := s.HandleRequest(context.Background(), "sess-1", req)

	if resp.Error == nil {
		t.Fatal("expected error for invalid version")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("expected code -32600, got %d", resp.Error.Code)
	}
}

func TestHandleRequest_MethodNotFound(t *testing.T) {
	s := &Server{sessions: make(map[string]*Session)}
	req := &Request{JSONRPC: "2.0", ID: json.RawMessage(`2`), Method: "unknown/method"}
	resp := s.HandleRequest(context.Background(), "sess-1", req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
}

func TestHandleRequest_Ping(t *testing.T) {
	s := &Server{sessions: make(map[string]*Session)}
	req := &Request{JSONRPC: "2.0", ID: json.RawMessage(`3`), Method: "ping"}
	resp := s.HandleRequest(context.Background(), "sess-1", req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("expected result for ping")
	}
}

func TestHandleRequest_Initialize(t *testing.T) {
	s := &Server{sessions: make(map[string]*Session)}

	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo:      Info{Name: "test", Version: "1.0"},
	})
	req := &Request{JSONRPC: "2.0", ID: json.RawMessage(`4`), Method: "initialize", Params: params}
	resp := s.HandleRequest(context.Background(), "sess-1", req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(InitializeResult)
	if !ok {
		t.Fatal("expected InitializeResult")
	}
	if result.ProtocolVersion != ProtocolVersion {
		t.Errorf("expected protocol %s, got %s", ProtocolVersion, result.ProtocolVersion)
	}
	if result.ServerInfo.Name != ServerName {
		t.Errorf("expected server name %s, got %s", ServerName, result.ServerInfo.Name)
	}
	if result.Capabilities.Tools == nil {
		t.Error("expected tools capability")
	}
}

func TestHandleRequest_Notification(t *testing.T) {
	s := &Server{sessions: make(map[string]*Session)}
	s.sessions["sess-1"] = &Session{ID: "sess-1"}

	req := &Request{JSONRPC: "2.0", Method: "notifications/initialized"}
	resp := s.HandleRequest(context.Background(), "sess-1", req)

	if resp != nil {
		t.Error("notifications should return nil response")
	}

	if !s.sessions["sess-1"].Initialized {
		t.Error("session should be marked as initialized")
	}
}

func TestSession_CreateAndDelete(t *testing.T) {
	s := &Server{sessions: make(map[string]*Session)}

	id := s.CreateSession()
	if id == "" {
		t.Fatal("expected non-empty session ID")
	}
	if !s.HasSession(id) {
		t.Error("session should exist after creation")
	}

	if !s.DeleteSession(id) {
		t.Error("delete should return true for existing session")
	}
	if s.HasSession(id) {
		t.Error("session should not exist after deletion")
	}
	if s.DeleteSession(id) {
		t.Error("delete should return false for non-existing session")
	}
}

func TestHandleRequest_ToolsCall_MissingName(t *testing.T) {
	s := &Server{sessions: make(map[string]*Session)}

	params, _ := json.Marshal(ToolCallParams{Name: ""})
	req := &Request{JSONRPC: "2.0", ID: json.RawMessage(`5`), Method: "tools/call", Params: params}
	resp := s.HandleRequest(context.Background(), "sess-1", req)

	if resp.Error == nil {
		t.Fatal("expected error for missing tool name")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected code -32602, got %d", resp.Error.Code)
	}
}

func TestHandleRequest_ResourcesRead_NotFound(t *testing.T) {
	s := &Server{sessions: make(map[string]*Session)}

	params, _ := json.Marshal(ResourceReadParams{URI: "duragraph://nonexistent"})
	req := &Request{JSONRPC: "2.0", ID: json.RawMessage(`6`), Method: "resources/read", Params: params}
	resp := s.HandleRequest(context.Background(), "sess-1", req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown resource")
	}
	if resp.Error.Code != -32002 {
		t.Errorf("expected code -32002, got %d", resp.Error.Code)
	}
}
