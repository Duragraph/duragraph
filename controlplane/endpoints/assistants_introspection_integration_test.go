package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestAssistantIntrospection(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE graphs, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	(&Server{Tenant: testPool}).RegisterAssistants(e.Group("/api/v1"))
	get := func(path string, status int) map[string]json.RawMessage {
		t.Helper()
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assistants/"+path, nil))
		if rec.Code != status {
			t.Fatalf("%s: want %d, got %d: %s", path, status, rec.Code, rec.Body.String())
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	id := seedAssistant(t, ctx, "introspection")
	empty := get(id+"/subgraphs", http.StatusOK)
	if len(empty) != 0 {
		t.Fatalf("no graph: want {}, got %v", empty)
	}
	get(id+"/schemas", http.StatusNotFound)
	get("not-a-uuid/schemas", http.StatusUnprocessableEntity)
	get("not-a-uuid/subgraphs", http.StatusUnprocessableEntity)
	get("11111111-1111-1111-1111-111111111111/schemas", http.StatusNotFound)
	get("11111111-1111-1111-1111-111111111111/subgraphs", http.StatusNotFound)

	nodes := `[{"id":"worker","type":"subgraph","config":{"namespace":"agent","input_schema":{"type":"object","title":"input"},"state_schema":{"type":"object"},"nodes":[{"id":"inner","type":"subgraph","config":{"output_schema":{"title":"nested"}}}] }},{"id":"bare","type":"subgraph","config":{}},{"id":"other","type":"tool"},{"type":"subgraph","config":{}},null]`
	if _, err := testPool.Exec(ctx, `INSERT INTO graphs (assistant_id, name, version, nodes, config) VALUES ($1, 'old', '1', '[]'::jsonb, '{}'::jsonb), ($1, 'new', '2', $2::jsonb, $3::jsonb)`, id, nodes, `{"input_schema":{"type":"object"},"output_schema":{"type":"string"},"state_schema":{"title":"state"},"context_schema":{"type":"object"}}`); err != nil {
		t.Fatal(err)
	}

	schemas := get(id+"/schemas", http.StatusOK)
	if string(schemas["graph_id"]) != `"new"` || string(schemas["state_schema"]) != `{"title":"state"}` || string(schemas["input_schema"]) != `{"type":"object"}` || string(schemas["context_schema"]) != `{"type":"object"}` {
		t.Fatalf("unexpected schemas: %v", schemas)
	}
	if _, ok := schemas["config_schema"]; ok {
		t.Fatalf("missing optional config schema must be omitted: %v", schemas)
	}

	subgraphs := get(id+"/subgraphs", http.StatusOK)
	if len(subgraphs) != 2 {
		t.Fatalf("non-recursive subgraphs: want 2, got %v", subgraphs)
	}
	var bare GraphSchemaNoId
	if err := json.Unmarshal(subgraphs["bare"], &bare); err != nil {
		t.Fatal(err)
	}
	if bare.InputSchema == nil || bare.OutputSchema == nil || bare.StateSchema == nil {
		t.Fatalf("required empty schemas: %+v", bare)
	}
	var agent map[string]json.RawMessage
	if err := json.Unmarshal(subgraphs["agent"], &agent); err != nil {
		t.Fatal(err)
	}
	if _, ok := agent["graph_id"]; ok {
		t.Fatalf("subgraph must not include graph_id: %v", agent)
	}
	if string(agent["input_schema"]) != `{"title":"input","type":"object"}` {
		t.Fatalf("input: %s", agent["input_schema"])
	}
	if len(get(id+"/subgraphs?recurse=true", http.StatusOK)) != 3 {
		t.Fatal("recurse must include nested subgraph")
	}
	filtered := get(id+"/subgraphs/agent?recurse=true", http.StatusOK)
	if len(filtered) != 2 || filtered["agent:inner"] == nil {
		t.Fatalf("namespace recursive filter: %v", filtered)
	}
	if len(get(id+"/subgraphs/agent", http.StatusOK)) != 1 {
		t.Fatal("namespace filter must return only root without recurse")
	}
	get(id+"/subgraphs/missing", http.StatusNotFound)
	get(id+"/subgraphs?recurse=invalid", http.StatusUnprocessableEntity)
}
