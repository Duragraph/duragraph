package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/mcp"
	"github.com/duragraph/duragraph/internal/infrastructure/tools"
	"github.com/labstack/echo/v4"
)

func TestMCPHandler_Post_InvalidContentType(t *testing.T) {
	registry := tools.NewRegistry()
	server := mcp.NewServer(registry, nil, nil, nil, nil, nil, nil)
	handler := NewMCPHandler(server)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Post(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", rec.Code)
	}
}

func TestMCPHandler_Post_ParseError(t *testing.T) {
	registry := tools.NewRegistry()
	server := mcp.NewServer(registry, nil, nil, nil, nil, nil, nil)
	handler := NewMCPHandler(server)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Post(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (JSON-RPC error), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "parse error") {
		t.Errorf("expected parse error in body, got %s", rec.Body.String())
	}
}

func TestMCPHandler_Post_Ping(t *testing.T) {
	registry := tools.NewRegistry()
	server := mcp.NewServer(registry, nil, nil, nil, nil, nil, nil)
	handler := NewMCPHandler(server)

	e := echo.New()
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Post(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get(mcpSessionHeader) == "" {
		t.Error("expected Mcp-Session-Id header in response")
	}
}

func TestMCPHandler_Get_NotAllowed(t *testing.T) {
	registry := tools.NewRegistry()
	server := mcp.NewServer(registry, nil, nil, nil, nil, nil, nil)
	handler := NewMCPHandler(server)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Get(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestMCPHandler_Delete_MissingSession(t *testing.T) {
	registry := tools.NewRegistry()
	server := mcp.NewServer(registry, nil, nil, nil, nil, nil, nil)
	handler := NewMCPHandler(server)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Delete(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestMCPHandler_Delete_NotFound(t *testing.T) {
	registry := tools.NewRegistry()
	server := mcp.NewServer(registry, nil, nil, nil, nil, nil, nil)
	handler := NewMCPHandler(server)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	req.Header.Set(mcpSessionHeader, "nonexistent-session")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Delete(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestMCPHandler_Delete_Success(t *testing.T) {
	registry := tools.NewRegistry()
	server := mcp.NewServer(registry, nil, nil, nil, nil, nil, nil)
	handler := NewMCPHandler(server)

	sessionID := server.CreateSession()

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	req.Header.Set(mcpSessionHeader, sessionID)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Delete(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
