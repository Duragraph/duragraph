package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/duragraph/duragraph/internal/infrastructure/mcp"
	"github.com/labstack/echo/v4"
)

const mcpSessionHeader = "Mcp-Session-Id"

type MCPHandler struct {
	server *mcp.Server
}

func NewMCPHandler(server *mcp.Server) *MCPHandler {
	return &MCPHandler{server: server}
}

// Post handles POST /mcp - main JSON-RPC endpoint
func (h *MCPHandler) Post(c echo.Context) error {
	contentType := c.Request().Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return c.JSON(http.StatusUnsupportedMediaType, map[string]string{
			"error": "Content-Type must be application/json",
		})
	}

	var req mcp.Request
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		resp := mcp.Response{
			JSONRPC: "2.0",
			Error:   &mcp.RPCError{Code: -32700, Message: "parse error"},
		}
		return c.JSON(http.StatusOK, resp)
	}

	// Get or create session
	sessionID := c.Request().Header.Get(mcpSessionHeader)
	if sessionID == "" {
		sessionID = h.server.CreateSession()
	}

	resp := h.server.HandleRequest(c.Request().Context(), sessionID, &req)

	// Notifications return no response
	if resp == nil {
		c.Response().Header().Set(mcpSessionHeader, sessionID)
		return c.NoContent(http.StatusAccepted)
	}

	c.Response().Header().Set(mcpSessionHeader, sessionID)
	return c.JSON(http.StatusOK, resp)
}

// Get handles GET /mcp - not supported for Streamable HTTP
func (h *MCPHandler) Get(c echo.Context) error {
	return c.JSON(http.StatusMethodNotAllowed, map[string]string{
		"error": "GET not supported; use POST for JSON-RPC requests",
	})
}

// Delete handles DELETE /mcp - terminate session
func (h *MCPHandler) Delete(c echo.Context) error {
	sessionID := c.Request().Header.Get(mcpSessionHeader)
	if sessionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Mcp-Session-Id header required",
		})
	}

	if h.server.DeleteSession(sessionID) {
		return c.NoContent(http.StatusOK)
	}

	return c.JSON(http.StatusNotFound, map[string]string{
		"error": "session not found",
	})
}
