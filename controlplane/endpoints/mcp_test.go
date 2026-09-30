package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func mcpTestRequest(e *echo.Echo, path, body, token string, headers map[string]string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Accept", "text/event-stream, application/json")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestMCPProtocolAndAuth(t *testing.T) {
	ctx := context.Background()
	resetMeTables(t, ctx)
	defer resetMeTables(t, ctx)
	uid := seedMeUser(t, ctx, "mcp-test@example.com", "user", "approved")
	var tid string
	if err := testPlatform.QueryRow(ctx, `INSERT INTO tenants(user_id,db_name,status) VALUES($1,$2,'approved') RETURNING id`, uid, testPool.Config().ConnConfig.Database).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	token := mintMeToken(t, uid, "mcp-test@example.com", "user", tid)
	s := &Server{Tenant: testPool, Platform: testPlatform, Auth: AuthConfig{JWTSecret: meTestSecret, BaseURL: "http://example.com"}}
	e := echo.New()
	s.RegisterMCP(e)
	init := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	for _, path := range []string{"/mcp", "/mcp/"} {
		rec := mcpTestRequest(e, path, init, token, nil)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		var result struct {
			ID     int `json:"id"`
			Result struct {
				ProtocolVersion string         `json:"protocolVersion"`
				Capabilities    map[string]any `json:"capabilities"`
			} `json:"result"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.ID != 0 || result.Result.ProtocolVersion != mcpVersion || result.Result.Capabilities["tools"] == nil {
			t.Fatalf("initialize: %s (%v)", rec.Body.String(), err)
		}
		if rec.Header().Get("Mcp-Session-Id") != "" {
			t.Fatal("stateless server must not issue sessions")
		}
	}
	for _, tc := range []struct {
		name, body string
		headers    map[string]string
		want       int
		content    string
	}{
		{"notification", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil, 202, ""},
		{"client response", `{"jsonrpc":"2.0","id":7,"result":{}}`, nil, 202, ""},
		{"ping", `{"jsonrpc":"2.0","id":"p","method":"ping"}`, nil, 200, `"result":{}`},
		{"unknown", `{"jsonrpc":"2.0","id":1,"method":"unknown"}`, nil, 200, `-32601`},
		{"bad JSON", `{`, nil, 400, `-32700`},
		{"batch", `[]`, nil, 400, `-32600`},
		{"null id", `{"jsonrpc":"2.0","id":null,"method":"ping"}`, nil, 400, `-32600`},
		{"wrong Accept", init, map[string]string{"Accept": "application/json"}, 400, ""},
		{"disabled SSE", init, map[string]string{"Accept": "application/json, text/event-stream;q=0.00"}, 400, ""},
		{"unsupported version", init, map[string]string{"MCP-Protocol-Version": "invalid"}, 400, ""},
		{"wrong content type", init, map[string]string{"Content-Type": "text/plain"}, 400, ""},
		{"foreign origin", init, map[string]string{"Origin": "https://evil.example"}, 403, ""},
		{"same origin", init, map[string]string{"Origin": "http://example.com"}, 200, `"protocolVersion"`},
		{"resources", `{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"duragraph://server/info"}}`, nil, 200, `duragraph://server/info`},
		{"tools", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil, 200, `"tools"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := mcpTestRequest(e, "/mcp", tc.body, token, tc.headers)
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.content) {
				t.Fatalf("want %d %q, got %d %s", tc.want, tc.content, rec.Code, rec.Body.String())
			}
		})
	}
	if rec := mcpTestRequest(e, "/mcp", init, "", nil); rec.Code != 401 {
		t.Fatalf("missing bearer: %d", rec.Code)
	}
	if rec := mcpTestRequest(e, "/mcp", init, "invalid", nil); rec.Code != 401 {
		t.Fatalf("bad bearer: %d", rec.Code)
	}
	// A valid signed token for a different tenant must never read this pool.
	other := mintMeToken(t, uid, "mcp-test@example.com", "user", "00000000-0000-0000-0000-000000000001")
	if rec := mcpTestRequest(e, "/mcp", init, other, nil); rec.Code != 403 {
		t.Fatalf("other tenant: %d", rec.Code)
	}
	if _, err := testPlatform.Exec(ctx, `UPDATE tenants SET db_name='different_tenant' WHERE id=$1`, tid); err != nil {
		t.Fatal(err)
	}
	if rec := mcpTestRequest(e, "/mcp", init, token, nil); rec.Code != 403 {
		t.Fatalf("other database: %d", rec.Code)
	}
	if _, err := testPlatform.Exec(ctx, `UPDATE tenants SET db_name=$1 WHERE id=$2`, testPool.Config().ConnConfig.Database, tid); err != nil {
		t.Fatal(err)
	}
	if _, err := testPlatform.Exec(ctx, `UPDATE users SET status='suspended' WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if rec := mcpTestRequest(e, "/mcp", init, token, nil); rec.Code != 403 {
		t.Fatalf("suspended user: %d", rec.Code)
	}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(method, "/mcp", nil))
		want := 405
		if method == http.MethodDelete {
			want = 404
		}
		if rec.Code != want {
			t.Fatalf("%s /mcp: %d", method, rec.Code)
		}
	}
}

func TestMCPAssistantInvocation(t *testing.T) {
	ctx := context.Background()
	resetMeTables(t, ctx)
	defer resetMeTables(t, ctx)
	uid := seedMeUser(t, ctx, "mcp-invoke@example.com", "user", "approved")
	var tid string
	if err := testPlatform.QueryRow(ctx, `INSERT INTO tenants(user_id,db_name,status) VALUES($1,$2,'approved') RETURNING id`, uid, testPool.Config().ConnConfig.Database).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	token := mintMeToken(t, uid, "mcp-invoke@example.com", "user", tid)
	id := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO assistants(id,name) VALUES ($1,'mcp assistant')`, id); err != nil {
		t.Fatal(err)
	}
	defer testPool.Exec(ctx, `DELETE FROM assistants WHERE id=$1`, id)
	s := &Server{Tenant: testPool, Platform: testPlatform, Auth: AuthConfig{JWTSecret: meTestSecret}}
	e := echo.New()
	s.RegisterMCP(e)
	list := mcpTestRequest(e, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, token, nil)
	if list.Code != 200 || !strings.Contains(list.Body.String(), "invoke_assistant_"+id) {
		t.Fatalf("tools/list: %d %s", list.Code, list.Body.String())
	}
	call := mcpTestRequest(e, "/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"invoke_assistant_`+id+`","arguments":{"input":{"question":"hi"}}}}`, token, nil)
	if call.Code != 200 || !strings.Contains(call.Body.String(), `"content"`) || strings.Contains(call.Body.String(), `"isError":true`) {
		t.Fatalf("tools/call: %d %s", call.Code, call.Body.String())
	}
	var runID string
	if err := testPool.QueryRow(ctx, `SELECT id::text FROM runs WHERE assistant_id=$1 AND input @> '{"question":"hi"}'::jsonb ORDER BY created_at DESC LIMIT 1`, id).Scan(&runID); err != nil {
		t.Fatalf("run not created: %v (%s)", err, call.Body.String())
	}
	if !strings.Contains(call.Body.String(), runID) {
		t.Fatalf("returned run ID mismatch: %s", call.Body.String())
	}
	if countRows(t, ctx, `SELECT count(*) FROM outbox WHERE event_type='run.created' AND aggregate_id=$1`, runID) != 1 {
		t.Fatal("run created without outbox event")
	}
	badConfig := mcpTestRequest(e, "/mcp", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"invoke_assistant_`+id+`","arguments":{"config":{"recursion_limit":1}}}}`, token, nil)
	if badConfig.Code != 200 || !strings.Contains(badConfig.Body.String(), `-32602`) {
		t.Fatalf("unsupported config: %d %s", badConfig.Code, badConfig.Body.String())
	}
	var threadID string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	threadCall := mcpTestRequest(e, "/mcp", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"invoke_assistant_`+id+`","arguments":{"thread_id":"`+threadID+`","input":{"question":"thread"}}}}`, token, nil)
	if threadCall.Code != 200 || !strings.Contains(threadCall.Body.String(), `"content"`) || strings.Contains(threadCall.Body.String(), `"isError":true`) {
		t.Fatalf("stateful call: %d %s", threadCall.Code, threadCall.Body.String())
	}
	if countRows(t, ctx, `SELECT count(*) FROM runs WHERE thread_id=$1 AND assistant_id=$2`, threadID, id) != 1 {
		t.Fatal("stateful run missing")
	}
	defer testPool.Exec(ctx, `DELETE FROM threads WHERE id=$1`, threadID)
	if _, err := testPool.Exec(ctx, `DELETE FROM runs WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
}
