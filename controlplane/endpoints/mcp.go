package endpoints

// Stateless Streamable HTTP MCP surface. Unlike the legacy implementation,
// this does not mint sessions or advertise tools it cannot execute.
import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	authpkg "github.com/duragraph/duragraph/internal/infrastructure/auth"
)

const mcpVersion = "2025-11-25"

type mcpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

func mcpReply(id json.RawMessage, result any, code int, message string) map[string]any {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	r := map[string]any{"jsonrpc": "2.0", "id": id}
	if code != 0 {
		r["error"] = map[string]any{"code": code, "message": message}
	} else {
		r["result"] = result
	}
	return r
}

func mcpMedia(h string, typ string) bool {
	for _, part := range strings.Split(h, ",") {
		media, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		q, hasQ := params["q"]
		quality, qErr := strconv.ParseFloat(q, 64)
		if err == nil && media == typ && (!hasQ || (qErr == nil && quality > 0 && quality <= 1)) {
			return true
		}
	}
	return false
}

// mcpAuthorize checks the live platform record, not the 24-hour JWT snapshot.
// The mounted tenant pool is a single database; never expose it to a token
// belonging to another tenant. A missing platform pool fails closed.
func (s *Server) mcpAuthorize(c echo.Context) error {
	if s.Platform == nil || s.Tenant == nil || len(s.Auth.JWTSecret) == 0 {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "MCP authentication unavailable")
	}
	// Only explicit bearer credentials: ambient cookies on a POST MCP tool call
	// would permit cross-site requests and are not MCP client authentication.
	token := bearerToken(c)
	if token == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "Bearer token required")
	}
	claims, err := authpkg.VerifyJWT(s.Auth.JWTSecret, token)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}
	uid, err := uuid.Parse(claims.UserID)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}
	var tenantID, dbName, userStatus, tenantStatus string
	err = s.Platform.QueryRow(c.Request().Context(), `SELECT t.id::text, t.db_name, u.status, t.status
		FROM users u JOIN tenants t ON t.user_id = u.id WHERE u.id = $1`, uid).
		Scan(&tenantID, &dbName, &userStatus, &tenantStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return echo.NewHTTPError(http.StatusUnauthorized, "unknown account")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "MCP authentication unavailable")
	}
	if claims.TenantID != tenantID || userStatus != "approved" || tenantStatus != "approved" ||
		dbName != s.Tenant.Config().ConnConfig.Database {
		return echo.NewHTTPError(http.StatusForbidden, "tenant access denied")
	}
	return nil
}

// RegisterMCP mounts only the POST contract. GET streaming is unsupported;
// DELETE cannot terminate a session on a stateless server.
func (s *Server) RegisterMCP(e *echo.Echo) {
	e.POST("/mcp", s.MCPPost)
	e.POST("/mcp/", s.MCPPost)
	e.GET("/mcp", func(c echo.Context) error { return c.NoContent(http.StatusMethodNotAllowed) })
	e.GET("/mcp/", func(c echo.Context) error { return c.NoContent(http.StatusMethodNotAllowed) })
	e.DELETE("/mcp", func(c echo.Context) error { return c.NoContent(http.StatusNotFound) })
	e.DELETE("/mcp/", func(c echo.Context) error { return c.NoContent(http.StatusNotFound) })
}

func (s *Server) MCPPost(c echo.Context) error {
	r := c.Request()
	if origin := r.Header.Get("Origin"); origin != "" {
		allowed := s.Auth.BaseURL
		if allowed == "" {
			// The Host header is attacker-controlled (DNS rebinding). A browser
			// Origin is safe only against an explicitly configured canonical URL.
			return echo.NewHTTPError(http.StatusForbidden, "MCP browser origin not configured")
		}
		u, err := url.Parse(allowed)
		if err != nil || origin != u.Scheme+"://"+u.Host {
			return echo.NewHTTPError(http.StatusForbidden, "invalid Origin")
		}
	}
	if err := s.mcpAuthorize(c); err != nil {
		return err
	}
	if !mcpMedia(r.Header.Get("Accept"), "application/json") || !mcpMedia(r.Header.Get("Accept"), "text/event-stream") {
		return echo.NewHTTPError(http.StatusBadRequest, "Accept must include application/json and text/event-stream")
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return echo.NewHTTPError(http.StatusBadRequest, "Content-Type must be application/json")
	}
	version := r.Header.Get("MCP-Protocol-Version")
	if version != "" && version != mcpVersion && version != "2025-06-18" && version != "2024-11-05" {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported MCP-Protocol-Version")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if !json.Valid(data) {
		return c.JSON(http.StatusBadRequest, mcpReply(nil, nil, -32700, "parse error"))
	}
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return c.JSON(http.StatusBadRequest, mcpReply(nil, nil, -32600, "invalid request"))
	}
	var msg mcpMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&msg); err != nil {
		return c.JSON(http.StatusBadRequest, mcpReply(nil, nil, -32700, "parse error"))
	}
	if dec.Decode(new(any)) != io.EOF {
		return c.JSON(http.StatusBadRequest, mcpReply(nil, nil, -32600, "invalid request"))
	}
	if msg.JSONRPC != "2.0" || (len(msg.ID) != 0 && !validMCPID(msg.ID)) ||
		(msg.Method == "" && len(msg.Result) == 0 && len(msg.Error) == 0) ||
		(msg.Method != "" && (len(msg.Result) != 0 || len(msg.Error) != 0)) {
		return c.JSON(http.StatusBadRequest, mcpReply(nil, nil, -32600, "invalid request"))
	}
	if msg.Method == "" { // client response; no server-initiated requests in stateless mode
		if len(msg.ID) == 0 {
			return c.JSON(http.StatusBadRequest, mcpReply(nil, nil, -32600, "invalid response"))
		}
		return c.NoContent(http.StatusAccepted)
	}
	if len(msg.ID) == 0 {
		return c.NoContent(http.StatusAccepted)
	}
	if msg.Method == "initialize" {
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(msg.Params, &params) != nil || params.ProtocolVersion == "" {
			return c.JSON(http.StatusOK, mcpReply(msg.ID, nil, -32602, "invalid initialize params"))
		}
		negotiated := mcpVersion
		if params.ProtocolVersion == "2025-06-18" || params.ProtocolVersion == "2024-11-05" {
			negotiated = params.ProtocolVersion
		}
		return c.JSON(http.StatusOK, mcpReply(msg.ID, map[string]any{
			"protocolVersion": negotiated, "capabilities": map[string]any{"resources": map[string]any{}, "tools": map[string]any{}},
			"serverInfo": map[string]string{"name": "duragraph", "version": Version},
		}, 0, ""))
	}
	result, code, text := s.mcpDispatch(c, msg)
	return c.JSON(http.StatusOK, mcpReply(msg.ID, result, code, text))
}

func validMCPID(raw json.RawMessage) bool {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	switch v.(type) {
	case string, float64:
		return true
	}
	return false
}

func (s *Server) mcpDispatch(c echo.Context, msg mcpMessage) (any, int, string) {
	switch msg.Method {
	case "ping":
		return map[string]any{}, 0, ""
	case "resources/list":
		return map[string]any{"resources": []any{map[string]string{"uri": "duragraph://assistants", "name": "Assistants", "mimeType": "application/json"}, map[string]string{"uri": "duragraph://server/info", "name": "Server Info", "mimeType": "application/json"}}}, 0, ""
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		if json.Unmarshal(msg.Params, &p) != nil || p.URI == "" {
			return nil, -32602, "invalid resource URI"
		}
		var data any
		switch p.URI {
		case "duragraph://server/info":
			data = map[string]string{"name": "duragraph", "version": Version, "protocol_version": mcpVersion}
		case "duragraph://assistants":
			assistants, err := s.mcpAssistants(c)
			if err != nil {
				return nil, -32603, "failed to list assistants"
			}
			data = assistants
		default:
			return nil, -32002, "resource not found"
		}
		b, _ := json.Marshal(data)
		return map[string]any{"contents": []any{map[string]string{"uri": p.URI, "mimeType": "application/json", "text": string(b)}}}, 0, ""
	case "tools/list":
		assistants, err := s.mcpAssistants(c)
		if err != nil {
			return nil, -32603, "failed to list assistants"
		}
		tools := make([]any, 0, len(assistants))
		for _, a := range assistants {
			tools = append(tools, map[string]any{"name": "invoke_assistant_" + a.ID, "description": "Queue a run for assistant " + a.Name,
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
					"input":     map[string]string{"type": "object"},
					"thread_id": map[string]string{"type": "string", "description": "Optional existing thread UUID"},
				}, "required": []string{}}})
		}
		return map[string]any{"tools": tools}, 0, ""
	case "tools/call":
		return s.mcpCall(c, msg.Params)
	default:
		return nil, -32601, "method not found"
	}
}

type mcpAssistant struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Metadata json.RawMessage `json:"metadata"`
}

func (s *Server) mcpAssistants(c echo.Context) ([]mcpAssistant, error) {
	rows, err := s.Tenant.Query(c.Request().Context(), `SELECT id::text, name, metadata FROM assistants ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []mcpAssistant{}
	for rows.Next() {
		var a mcpAssistant
		if err := rows.Scan(&a.ID, &a.Name, &a.Metadata); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Server) mcpCall(c echo.Context, raw json.RawMessage) (any, int, string) {
	var p struct {
		Name      string `json:"name"`
		Arguments struct {
			Input    map[string]any `json:"input"`
			ThreadID string         `json:"thread_id"`
			Config   map[string]any `json:"config"`
		} `json:"arguments"`
	}
	if json.Unmarshal(raw, &p) != nil || !strings.HasPrefix(p.Name, "invoke_assistant_") {
		return nil, -32602, "invalid tool call"
	}
	id, err := uuid.Parse(strings.TrimPrefix(p.Name, "invoke_assistant_"))
	if err != nil {
		return nil, -32602, "invalid assistant ID"
	}
	if p.Arguments.ThreadID != "" {
		if _, err := uuid.Parse(p.Arguments.ThreadID); err != nil {
			return nil, -32602, "invalid thread ID"
		}
	}
	if len(p.Arguments.Config) != 0 {
		return nil, -32602, "run configuration is not supported by v2"
	}
	ctx := c.Request().Context()
	var exists bool
	if err := s.Tenant.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assistants WHERE id=$1)`, id).Scan(&exists); err != nil {
		return nil, -32603, "failed to check assistant"
	}
	if !exists {
		return map[string]any{"content": []any{map[string]string{"type": "text", "text": "assistant not found"}}, "isError": true}, 0, ""
	}
	// Reuse v2's run creation path, including transactional outbox and validation;
	// never write a run directly from the protocol adapter.
	request, _ := json.Marshal(map[string]any{"assistant_id": id.String(), "input": p.Arguments.Input, "config": p.Arguments.Config})
	childReq := c.Request().Clone(ctx)
	childReq.Body = io.NopCloser(bytes.NewReader(request))
	childReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	child := c.Echo().NewContext(childReq, &mcpResponseWriter{header: make(http.Header)})
	var runErr error
	if p.Arguments.ThreadID != "" {
		child.SetParamNames("id")
		child.SetParamValues(p.Arguments.ThreadID)
		runErr = s.RunsCreateOnThread(child)
	} else {
		runErr = s.RunsCreateStateless(child)
	}
	if runErr != nil {
		return map[string]any{"content": []any{map[string]string{"type": "text", "text": "failed to create run"}}, "isError": true}, 0, ""
	}
	writer := child.Response().Writer.(*mcpResponseWriter)
	var run struct {
		RunID uuid.UUID `json:"run_id"`
	}
	if json.Unmarshal(writer.body.Bytes(), &run) != nil {
		return nil, -32603, "failed to read run"
	}
	b, _ := json.Marshal(map[string]string{"run_id": run.RunID.String(), "assistant_id": id.String(), "status": "queued"})
	return map[string]any{"content": []any{map[string]string{"type": "text", "text": string(b)}}}, 0, ""
}

type mcpResponseWriter struct {
	header http.Header
	body   bytes.Buffer
}

func (w *mcpResponseWriter) Header() http.Header         { return w.header }
func (w *mcpResponseWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *mcpResponseWriter) WriteHeader(int)             {}
