# Layer 3 Stubs Pass 1 (store + system) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `store` (5 endpoints) and `system` (`/ok`, `/info`, `/metrics`) control-plane endpoint groups, and add a reusable generator `custom` flag so hand-written handlers coexist with generated routes.

**Architecture:** `store` is a non-event-sourced group whose handlers run plain `s.Tenant` pool queries; all five are hand-written (`store.go`) with generated route registration only. `system` is fully hand-written (`system.go`), mounted on the root Echo instance (not `/api/v1`). The single template change is a `custom: true` endpoint flag: the generator emits the route inside `Register<Group>` but skips the method body, which lives in a sibling `package endpoints` file.

**Tech Stack:** Go 1.23, Echo v4, pgx v5, testcontainers-go (Postgres), prometheus/client_golang, oapi-codegen types (`types_gen.go`), text/template generator.

## Global Constraints

- Source of truth is `spec/models/d2/` (the d2 diagrams) and the API contract in `spec/api/duragraph-latest.yaml`. Build FROM the spec; never reconcile back to the old `internal/` tree.
- `store_items.namespace` is `TEXT[]` (postgres.d2) / `[]string` (OpenAPI). Use Postgres array operators, NOT `LIKE`. The `LIKE :prefix` SQL in `endpoint-queries.d2` is stale and discarded.
- Generated files carry the header `// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.` — never hand-edit `*_gen.go`; change the manifest/template and regenerate (`go run ./controlplane/gen` from the module root `~/worktrees/duragraph/feat/controlplane-server`).
- Integration tests use testcontainers and run in standard CI — no `//go:build integration` tag. Tests are part of "done".
- Handler conventions: `echo.NewHTTPError(status, msg)`; `pgx.ErrNoRows` → 404; bind failure → 400. Match the exact HTTP status codes declared in `duragraph-latest.yaml` (PUT/DELETE `/store/items` → 204; GET → 200/400; search + namespaces → 200).
- Never hand-edit `go.mod`/`go.sum` (`prometheus/client_golang` is already present — no change needed).
- Conventional commits. Commit after each task. Do not open a PR or merge without explicit per-PR approval.
- Run all commands from the worktree root: `~/worktrees/duragraph/feat/controlplane-server`.

---

## File Structure

- `controlplane/endpoints/rows.go` (modify) — add `storeItemRow` + `toAPI()`; extend DIVERGENCES.
- `controlplane/gen/main.go` (modify) — parse `custom` on `endpoint`.
- `controlplane/gen/templates/group.go.tmpl` (modify) — skip method body for custom endpoints.
- `controlplane/gen/endpoints.yaml` (modify) — mark 5 store endpoints `custom: true`; remove `system` group.
- `controlplane/endpoints/store_gen.go` (regenerate) — `RegisterStore` routes only.
- `controlplane/endpoints/store.go` (create) — 5 hand-written store handlers.
- `controlplane/endpoints/system_gen.go` (delete).
- `controlplane/endpoints/system.go` (create) — `RegisterSystem(e)` + 3 handlers.
- `controlplane/server/server.go` (modify) — mount system on root `e`; set process start time.
- `controlplane/endpoints/store_integration_test.go` (create).
- `controlplane/endpoints/system_integration_test.go` (create).
- `spec/models/d2/endpoint-queries.d2` (modify, spec repo) — reconcile store SQL to `TEXT[]` array ops.

---

## Task 1: `storeItemRow` + `toAPI` mapper

**Files:**
- Modify: `controlplane/endpoints/rows.go`
- Test: `controlplane/endpoints/rows_store_test.go` (create)

**Interfaces:**
- Consumes: `Item` (from `types_gen.go`: `{Namespace []string, Key string, Value map[string]interface{}, CreatedAt, UpdatedAt time.Time}`).
- Produces: `type storeItemRow struct{...}` with `db` tags; method `func (r storeItemRow) toAPI() Item`. Used by Task 2's store handlers.

- [ ] **Step 1: Write the failing test**

Create `controlplane/endpoints/rows_store_test.go`:

```go
package endpoints

import (
	"testing"
	"time"
)

func TestStoreItemRowToAPI(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	r := storeItemRow{
		ID:        7,
		Namespace: []string{"users", "42"},
		Key:       "profile",
		Value:     []byte(`{"name":"ada"}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
	got := r.toAPI()
	if got.Key != "profile" {
		t.Errorf("key: want profile, got %q", got.Key)
	}
	if len(got.Namespace) != 2 || got.Namespace[0] != "users" || got.Namespace[1] != "42" {
		t.Errorf("namespace: want [users 42], got %v", got.Namespace)
	}
	if got.Value["name"] != "ada" {
		t.Errorf("value.name: want ada, got %v", got.Value["name"])
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Errorf("timestamps not mapped: %v / %v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestStoreItemRowToAPIEmptyValue(t *testing.T) {
	r := storeItemRow{Namespace: []string{"a"}, Key: "k"}
	got := r.toAPI()
	if got.Value == nil {
		t.Fatal("value should be non-nil empty map, got nil")
	}
	if len(got.Value) != 0 {
		t.Errorf("value: want empty, got %v", got.Value)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/endpoints/ -run TestStoreItemRow -v`
Expected: FAIL — compile error `undefined: storeItemRow`.

- [ ] **Step 3: Add the row struct + mapper**

Append to `controlplane/endpoints/rows.go` (before the DIVERGENCES comment block):

```go
// storeItemRow mirrors the store_items table (postgres.d2 store_ctx). namespace
// is TEXT[] (a hierarchical path), value is jsonb. Not event-sourced.
type storeItemRow struct {
	ID        int64     `db:"id"`
	Namespace []string  `db:"namespace"`
	Key       string    `db:"key"`
	Value     []byte    `db:"value"` // jsonb
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// toAPI maps a store_items row to the OpenAPI Item response type. value is
// jsonb, best-effort unmarshalled into the document map (empty map when null).
func (r storeItemRow) toAPI() Item {
	it := Item{
		Namespace: r.Namespace,
		Key:       r.Key,
		Value:     map[string]interface{}{},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if len(r.Value) > 0 {
		_ = json.Unmarshal(r.Value, &it.Value)
	}
	return it
}
```

- [ ] **Step 4: Extend the DIVERGENCES block**

In `controlplane/endpoints/rows.go`, inside the trailing `// DIVERGENCES (...)` comment, add these lines at the end:

```go
//   store: namespace is TEXT[] (postgres.d2) / []string (OpenAPI Item.Namespace)
//     — the endpoint-queries.d2 `namespace LIKE :prefix` SQL is stale and
//     discarded in favor of array ops. StoreSearchRequest.query (vector/semantic
//     search) is not honored — no vector index in the new control plane yet;
//     filter + namespace_prefix only. StoreListNamespacesRequest.max_depth and
//     .suffix are best-effort (prefix + limit/offset honored).
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./controlplane/endpoints/ -run TestStoreItemRow -v`
Expected: PASS (both subtests).

- [ ] **Step 6: Commit**

```bash
git add controlplane/endpoints/rows.go controlplane/endpoints/rows_store_test.go
git commit -m "feat(controlplane): storeItemRow + toAPI mapper for store group"
```

---

## Task 2: Generator `custom` flag + store handlers

Marks the 5 store endpoints `custom` (routes generated, bodies hand-written) and implements them. The package only compiles when the routes-only generated file and the hand-written bodies exist together, so both land in this task, gated by the store integration test.

**Files:**
- Modify: `controlplane/gen/main.go`
- Modify: `controlplane/gen/templates/group.go.tmpl`
- Modify: `controlplane/gen/endpoints.yaml`
- Regenerate: `controlplane/endpoints/store_gen.go`
- Create: `controlplane/endpoints/store.go`
- Test: `controlplane/endpoints/store_integration_test.go`

**Interfaces:**
- Consumes: `storeItemRow`, `storeItemRow.toAPI()` (Task 1); `Server{Tenant *pgxpool.Pool}`, `deref`, `intOr` (runtime.go); OpenAPI `StorePutRequest`, `StoreDeleteRequest`, `StoreSearchRequest`, `StoreListNamespacesRequest`, `Item`, `SearchItemsResponse`, `ListNamespaceResponse` (types_gen.go).
- Produces: methods `func (s *Server) StorePut|StoreGet|StoreDelete|StoreSearch|StoreListNamespaces(c echo.Context) error`; generated `func (s *Server) RegisterStore(g *echo.Group)`.

- [ ] **Step 1: Write the failing test**

Create `controlplane/endpoints/store_integration_test.go`:

```go
package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newTestServerWithStore() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	g := e.Group("/api/v1")
	s.RegisterStore(g)
	return e
}

func doJSON(t *testing.T, e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	return rec
}

func TestStoreCRUD(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE store_items, store_namespaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithStore()

	// put (create)
	if rec := doJSON(t, e, http.MethodPut, "/api/v1/store/items",
		`{"namespace":["users","42"],"key":"profile","value":{"name":"ada"}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("put: want 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// get
	rec := doJSON(t, e, http.MethodGet, "/api/v1/store/items?namespace=users&namespace=42&key=profile", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var item Item
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if item.Value["name"] != "ada" {
		t.Errorf("get value.name: want ada, got %v", item.Value["name"])
	}

	// put (upsert overwrites)
	if rec := doJSON(t, e, http.MethodPut, "/api/v1/store/items",
		`{"namespace":["users","42"],"key":"profile","value":{"name":"grace"}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("upsert: want 204, got %d", rec.Code)
	}
	rec = doJSON(t, e, http.MethodGet, "/api/v1/store/items?namespace=users&namespace=42&key=profile", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &item)
	if item.Value["name"] != "grace" {
		t.Errorf("after upsert: want grace, got %v", item.Value["name"])
	}

	// get missing -> 404
	if rec := doJSON(t, e, http.MethodGet, "/api/v1/store/items?namespace=users&namespace=42&key=nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("get missing: want 404, got %d", rec.Code)
	}

	// search: prefix + value filter
	_ = doJSON(t, e, http.MethodPut, "/api/v1/store/items",
		`{"namespace":["users","99"],"key":"profile","value":{"name":"linus","team":"kernel"}}`)
	rec = doJSON(t, e, http.MethodPost, "/api/v1/store/items/search",
		`{"namespace_prefix":["users"],"filter":{"team":"kernel"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("search: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var sr SearchItemsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &sr); err != nil {
		t.Fatalf("search decode: %v", err)
	}
	if len(sr.Items) != 1 || sr.Items[0].Key != "profile" || sr.Items[0].Namespace[1] != "99" {
		t.Errorf("search: want 1 item ns[users 99], got %+v", sr.Items)
	}

	// list_namespaces under prefix
	rec = doJSON(t, e, http.MethodPost, "/api/v1/store/namespaces", `{"prefix":["users"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("namespaces: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var ns ListNamespaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &ns); err != nil {
		t.Fatalf("namespaces decode: %v", err)
	}
	if len(ns) != 2 {
		t.Errorf("namespaces: want 2 distinct, got %d: %v", len(ns), ns)
	}

	// delete -> 204, then get 404
	if rec := doJSON(t, e, http.MethodDelete, "/api/v1/store/items",
		`{"namespace":["users","42"],"key":"profile"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", rec.Code)
	}
	if rec := doJSON(t, e, http.MethodGet, "/api/v1/store/items?namespace=users&namespace=42&key=profile", ""); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete: want 404, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/endpoints/ -run TestStoreCRUD -v`
Expected: FAIL — the store handlers return the stub `map[string]any{}` (put/delete give 200 not 204), so assertions fail. (Package compiles; store_gen.go still has stub bodies.)

- [ ] **Step 3: Add the `Custom` field to the generator's endpoint model**

In `controlplane/gen/main.go`, add a field to the `endpoint` struct (after `Response`):

```go
	Response string   `yaml:"response"`
	Custom   bool     `yaml:"custom"` // hand-written body in <group>.go; generate route only
	Impl     *impl    `yaml:"impl"`
```

- [ ] **Step 4: Skip the method body for custom endpoints in the template**

In `controlplane/gen/templates/group.go.tmpl`, wrap the per-endpoint method block so custom endpoints emit no body. Change the block that starts at `{{range .Endpoints}}` (the one right after the `Register` func, beginning with the `// {{.Handler}} — ...` comment) to open with a guard and close it:

Replace this opening:

```
{{range .Endpoints}}
// {{.Handler}} — {{.Method}} {{.Path}}  (kind: {{.Kind}})
```

with:

```
{{range .Endpoints}}
{{- if .Custom}}
// {{.Handler}} — {{.Method}} {{.Path}}  (kind: {{.Kind}}) — hand-written in {{$.Name}}.go
{{- else}}
// {{.Handler}} — {{.Method}} {{.Path}}  (kind: {{.Kind}})
```

and replace the block's closing (the final `}` of the handler func immediately before `{{end}}` at the very end of the file):

```
	return echo.NewHTTPError(http.StatusNotImplemented, "handler not implemented")
{{- end}}
}
{{end}}
```

with:

```
	return echo.NewHTTPError(http.StatusNotImplemented, "handler not implemented")
{{- end}}
}
{{- end}}
{{end}}
```

(The added outer `{{- if .Custom}} ... {{- else}} ... {{- end}}` emits only the doc comment for custom endpoints and skips the `func`. The `Register` loop at the top is unchanged, so routes are still generated for every endpoint.)

- [ ] **Step 5: Mark the store endpoints custom in the manifest**

In `controlplane/gen/endpoints.yaml`, under `- name: store`, add `custom: true` to each of the five endpoints (`put`, `get`, `delete`, `search`, `list_namespaces`). Example for the first; apply the same `custom: true` line to all five:

```yaml
- name: store
  db: tenant
  endpoints:
  - name: put
    method: PUT
    path: /store/items
    kind: write
    outbox: false
    request: StorePutRequest
    custom: true
  - name: get
    method: GET
    path: /store/items
    kind: read
    outbox: false
    response: Item
    custom: true
  - name: delete
    method: DELETE
    path: /store/items
    kind: delete
    outbox: false
    request: StoreDeleteRequest
    custom: true
  - name: search
    method: POST
    path: /store/items/search
    kind: read
    outbox: false
    request: StoreSearchRequest
    response: SearchItemsResponse
    custom: true
  - name: list_namespaces
    method: POST
    path: /store/namespaces
    kind: read
    outbox: false
    request: StoreListNamespacesRequest
    custom: true
```

(Preserve any existing `steps:` lists on these endpoints — only add the `custom: true` line.)

- [ ] **Step 6: Regenerate and confirm the done groups are untouched**

Run: `go run ./controlplane/gen`
Expected output includes `✓ store_gen.go (5 endpoints)`.

Run: `git diff --stat controlplane/endpoints/assistants_gen.go controlplane/endpoints/threads_gen.go controlplane/endpoints/runs_gen.go controlplane/endpoints/crons_gen.go`
Expected: no output (the 4 implemented groups regenerate byte-identical — the template guard only affects custom endpoints).

Run: `git diff controlplane/endpoints/store_gen.go`
Expected: the 5 stub method bodies are gone; `RegisterStore` still registers all 5 routes; only `echo` is imported.

- [ ] **Step 7: Write the store handlers**

Create `controlplane/endpoints/store.go`:

```go
// Hand-written store handlers. store is the LangGraph cross-thread KV store
// (store_items table, per-tenant DB) — NOT event-sourced, so these run plain
// s.Tenant pool queries with no events/outbox/writeTx. Routes are generated
// into store_gen.go (endpoints marked custom: true in endpoints.yaml); the
// bodies live here. namespace is TEXT[] — array ops, never LIKE.
package endpoints

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// StorePut upserts an item by (namespace, key). PUT /store/items -> 204.
func (s *Server) StorePut(c echo.Context) error {
	ctx := c.Request().Context()
	var req StorePutRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	value := mustJSON(req.Value)
	if _, err := s.Tenant.Exec(ctx, `
		INSERT INTO store_items (namespace, key, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (namespace, key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		req.Namespace, req.Key, value); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// StoreGet reads one item by namespace (repeated query param) + key.
// GET /store/items?namespace=a&namespace=b&key=k -> 200 Item / 404.
func (s *Server) StoreGet(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.QueryParams()["namespace"]
	key := c.QueryParam("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, namespace, key, value, created_at, updated_at
		FROM store_items WHERE namespace = $1 AND key = $2`, namespace, key)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[storeItemRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// StoreDelete removes an item by (namespace, key). DELETE /store/items -> 204
// (idempotent; 204 even when nothing matched, per the OpenAPI contract).
func (s *Server) StoreDelete(c echo.Context) error {
	ctx := c.Request().Context()
	var req StoreDeleteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var namespace []string
	if req.Namespace != nil {
		namespace = *req.Namespace
	}
	if _, err := s.Tenant.Exec(ctx,
		`DELETE FROM store_items WHERE namespace = $1 AND key = $2`,
		namespace, req.Key); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// StoreSearch lists items under a namespace prefix, optionally filtered by a
// jsonb subset match. POST /store/items/search -> 200 SearchItemsResponse.
// namespace_prefix matches when the item's leading namespace elements equal the
// prefix array. filter uses `value @> $filter` (GIN jsonb_path_ops index).
func (s *Server) StoreSearch(c echo.Context) error {
	ctx := c.Request().Context()
	var req StoreSearchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var prefix []string
	if req.NamespacePrefix != nil {
		prefix = *req.NamespacePrefix
	}
	var filter any
	if req.Filter != nil {
		filter = mustJSON(*req.Filter)
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, namespace, key, value, created_at, updated_at
		FROM store_items
		WHERE ($1::text[] IS NULL OR namespace[1:cardinality($1)] = $1)
		  AND ($2::jsonb IS NULL OR value @> $2)
		ORDER BY namespace, key
		LIMIT $3 OFFSET $4`,
		nilIfEmpty(prefix), filter, intOr(req.Limit, 10), intOr(req.Offset, 0))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[storeItemRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := SearchItemsResponse{Items: make([]Item, len(list))}
	for i := range list {
		out.Items[i] = list[i].toAPI()
	}
	return c.JSON(http.StatusOK, out)
}

// StoreListNamespaces returns the distinct namespaces under a prefix.
// POST /store/namespaces -> 200 ListNamespaceResponse ([][]string).
func (s *Server) StoreListNamespaces(c echo.Context) error {
	ctx := c.Request().Context()
	var req StoreListNamespacesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var prefix []string
	if req.Prefix != nil {
		prefix = *req.Prefix
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT DISTINCT namespace
		FROM store_items
		WHERE ($1::text[] IS NULL OR namespace[1:cardinality($1)] = $1)
		ORDER BY namespace
		LIMIT $2 OFFSET $3`,
		nilIfEmpty(prefix), intOr(req.Limit, 100), intOr(req.Offset, 0))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := ListNamespaceResponse{}
	for rows.Next() {
		var ns []string
		if err := rows.Scan(&ns); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		out = append(out, ns)
	}
	if rows.Err() != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, rows.Err().Error())
	}
	return c.JSON(http.StatusOK, out)
}

func (s *Server) storeUnused() { _ = json.Marshal } // placeholder removed in Step 9
```

- [ ] **Step 8: Add the `nilIfEmpty` helper**

Append to `controlplane/endpoints/runtime.go`:

```go
// nilIfEmpty returns nil (SQL NULL) for an empty/absent string slice so a
// `$n::text[] IS NULL OR ...` guard clause matches everything. A non-empty
// slice is returned as-is for a TEXT[] bind.
func nilIfEmpty(s []string) any {
	if len(s) == 0 {
		return nil
	}
	return s
}
```

- [ ] **Step 9: Remove the placeholder line**

Delete the final `func (s *Server) storeUnused()...` line and the now-unused `"encoding/json"` import from `controlplane/endpoints/store.go` (json is not used by the handlers directly). Run `goimports`/`gofmt` implicitly via the build in the next step.

- [ ] **Step 10: Build, then run the test to verify it passes**

Run: `go build ./controlplane/...`
Expected: no output (compiles).

Run: `go test ./controlplane/endpoints/ -run TestStoreCRUD -v`
Expected: PASS.

- [ ] **Step 11: Run the whole endpoints package (regression guard)**

Run: `go test ./controlplane/endpoints/ -v`
Expected: PASS — store + all previously-implemented groups (assistants/threads/runs/crons) still green.

- [ ] **Step 12: Commit**

```bash
git add controlplane/gen/main.go controlplane/gen/templates/group.go.tmpl \
  controlplane/gen/endpoints.yaml controlplane/endpoints/store_gen.go \
  controlplane/endpoints/store.go controlplane/endpoints/runtime.go \
  controlplane/endpoints/store_integration_test.go
git commit -m "feat(controlplane): store group + generator custom-endpoint flag"
```

---

## Task 3: `system` group (root-mounted, hand-written)

**Files:**
- Modify: `controlplane/gen/endpoints.yaml` (remove the `system` group)
- Delete: `controlplane/endpoints/system_gen.go`
- Create: `controlplane/endpoints/system.go`
- Modify: `controlplane/server/server.go`
- Test: `controlplane/endpoints/system_integration_test.go`

**Interfaces:**
- Consumes: `Server{Tenant *pgxpool.Pool}`; `promhttp.Handler()` (`github.com/prometheus/client_golang/prometheus/promhttp`).
- Produces: `func (s *Server) RegisterSystem(e *echo.Echo)`; package vars `Version`, `GitSHA` (settable via `-ldflags -X`). `RegisterSystem` takes the ROOT `*echo.Echo`, not an `*echo.Group`.

- [ ] **Step 1: Write the failing test**

Create `controlplane/endpoints/system_integration_test.go`:

```go
package endpoints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newTestServerWithSystem() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	s.RegisterSystem(e) // root mount, NOT /api/v1
	return e
}

func TestSystemOK(t *testing.T) {
	e := newTestServerWithSystem()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/ok: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("/ok decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`/ok: want status "ok", got %q`, body["status"])
	}
}

func TestSystemInfo(t *testing.T) {
	e := newTestServerWithSystem()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/info", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/info: want 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("/info decode: %v", err)
	}
	for _, k := range []string{"version", "git_sha", "uptime_seconds"} {
		if _, ok := body[k]; !ok {
			t.Errorf("/info missing %q: %v", k, body)
		}
	}
}

func TestSystemMetrics(t *testing.T) {
	e := newTestServerWithSystem()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Errorf("/metrics: expected prometheus text with go_goroutines, got first 200 bytes: %.200s", rec.Body.String())
	}
}

func TestSystemNotUnderAPIV1(t *testing.T) {
	e := newTestServerWithSystem()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ok", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("/api/v1/ok should not exist: want 404, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/endpoints/ -run TestSystem -v`
Expected: FAIL — `RegisterSystem` currently takes an `*echo.Group` (generated), so `s.RegisterSystem(e)` is a compile error / signature mismatch.

- [ ] **Step 3: Remove the system group from the generator manifest**

In `controlplane/gen/endpoints.yaml`, delete the entire `- name: system` group block (all three endpoints: health/info/metrics).

- [ ] **Step 4: Delete the generated system file**

Run: `git rm controlplane/endpoints/system_gen.go`

- [ ] **Step 5: Regenerate and confirm system is no longer produced**

Run: `go run ./controlplane/gen`
Expected: output no longer lists `system_gen.go`; `store_gen.go` and the 4 done groups still listed.

Run: `test ! -f controlplane/endpoints/system_gen.go && echo "gone"`
Expected: `gone`.

- [ ] **Step 6: Write the hand-written system handlers**

Create `controlplane/endpoints/system.go`:

```go
// Hand-written system endpoints (health/info/metrics). Not generated and not
// under /api/v1 — RegisterSystem mounts on the ROOT Echo instance because the
// spec paths are root-level (/ok, /info, /metrics) and the dashboard auth gate
// expects /info at root. Source of truth: spec/models/d2/endpoint-queries.d2
// (system_ep).
package endpoints

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Build metadata, overridden at link time with -ldflags "-X ...Version=... -X ...GitSHA=...".
var (
	Version = "dev"
	GitSHA  = "none"
)

// processStart anchors /info uptime; captured at process start.
var processStart = time.Now()

// RegisterSystem mounts /ok, /info, /metrics on the root Echo instance.
func (s *Server) RegisterSystem(e *echo.Echo) {
	e.GET("/ok", s.SystemOK)
	e.GET("/info", s.SystemInfo)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
}

// SystemOK is the readiness probe: DB reachable -> 200, else 503.
func (s *Server) SystemOK(c echo.Context) error {
	ctx := c.Request().Context()
	if s.Tenant != nil {
		if err := s.Tenant.Ping(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
				"error":  err.Error(),
			})
		}
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SystemInfo returns build + uptime info (in-memory, no DB).
func (s *Server) SystemInfo(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"version":        Version,
		"git_sha":        GitSHA,
		"uptime_seconds": int64(time.Since(processStart).Seconds()),
	})
}
```

- [ ] **Step 7: Mount system on the root in the composition root**

In `controlplane/server/server.go`, find the endpoint registration block:

```go
	g := e.Group("/api/v1")
	ep.RegisterAssistants(g)
	...
	ep.RegisterAdmin(g)
	ep.RegisterSystem(g)
```

Change the system line to mount on the root `e` instead of the `/api/v1` group `g`:

```go
	g := e.Group("/api/v1")
	ep.RegisterAssistants(g)
	...
	ep.RegisterAdmin(g)
	ep.RegisterSystem(e) // root-level: /ok, /info, /metrics
```

- [ ] **Step 8: Build and run the system tests**

Run: `go build ./controlplane/...`
Expected: no output.

Run: `go test ./controlplane/endpoints/ -run TestSystem -v`
Expected: PASS (all four subtests).

- [ ] **Step 9: Full package + server package regression**

Run: `go test ./controlplane/endpoints/ ./controlplane/server/ -v`
Expected: PASS — server assembly still builds and mounts everything (the server integration test still passes with system on root).

- [ ] **Step 10: Commit**

```bash
git add controlplane/gen/endpoints.yaml controlplane/endpoints/system.go \
  controlplane/endpoints/system_integration_test.go controlplane/server/server.go
git add -u controlplane/endpoints/system_gen.go
git commit -m "feat(controlplane): system endpoints (/ok,/info,/metrics) mounted at root"
```

---

## Task 4: Reconcile store SQL in the spec diagram

Brings `endpoint-queries.d2` (the query-mapping diagram) in line with the `TEXT[]` schema the implementation uses, satisfying spec-first hygiene. This edits the spec repo (the symlinked `spec/`), a separate git repository.

**Files:**
- Modify: `spec/models/d2/endpoint-queries.d2` (spec repo, branch `spec/models-diagrams`)

**Interfaces:** none (documentation/spec).

- [ ] **Step 1: Update the store query mappings to array ops**

In `spec/models/d2/endpoint-queries.d2`, in the `store:` block, replace the `LIKE :prefix || '%'` mappings with `TEXT[]` array-prefix semantics. Specifically:

- `search` step 1 →
  `"1. SELECT *": "FROM store_items WHERE namespace[1:cardinality(:prefix)] = :prefix AND value @> :filter ORDER BY namespace, key LIMIT :limit OFFSET :offset"`
- `list_namespaces` step 1 →
  `"1. SELECT DISTINCT namespace": "FROM store_items WHERE namespace[1:cardinality(:prefix)] = :prefix"`

Add a note line in the `store:` block header comment area:
`# namespace is TEXT[] (postgres.d2 store_ctx) — array-prefix match, not LIKE.`

- [ ] **Step 2: Verify the d2 still parses (if d2 CLI is available)**

Run: `command -v d2 >/dev/null && d2 spec/models/d2/endpoint-queries.d2 /tmp/eq.svg && echo "d2 OK" || echo "d2 not installed — skipping render check"`
Expected: `d2 OK` (or the skip message if d2 isn't installed — the edit is plain text either way).

- [ ] **Step 3: Commit in the spec repo**

```bash
git -C /home/qwe/platform/duragraph-org/spec add models/d2/endpoint-queries.d2
git -C /home/qwe/platform/duragraph-org/spec commit -m "spec(models): store queries use TEXT[] array-prefix, not LIKE"
```

---

## Self-Review

**Spec coverage:**
- store put/get/delete/search/list_namespaces → Task 2 (handlers + integration test). ✓
- store namespace TEXT[] decision → Task 1 (DIVERGENCES) + Task 2 (array-op SQL) + Task 4 (d2). ✓
- Generator `custom` flag → Task 2 (main.go + template + manifest). ✓
- system /ok, /info, /metrics, root mount → Task 3. ✓
- Testcontainers integration tests per group → Task 2 + Task 3. ✓
- Deferred groups (workers/admin/platform/auth) → not in plan (correct; out of scope per spec). ✓

**Placeholder scan:** No TBD/TODO-as-requirement. Every code step shows full code. The Step 8/9 `storeUnused` placeholder is explicitly created and then explicitly deleted with instructions — not a dangling stub.

**Type consistency:** `storeItemRow` fields (`ID int64, Namespace []string, Key string, Value []byte, CreatedAt/UpdatedAt time.Time`) match `toAPI()` and the `store_items` columns. Handler names `StorePut/StoreGet/StoreDelete/StoreSearch/StoreListNamespaces` = `pascal(store)+pascal(<endpoint name>)`, matching the generator's `Handler` computation for manifest names `put/get/delete/search/list_namespaces`. `RegisterSystem(e *echo.Echo)` signature is consistent between system.go, the test, and server.go. `nilIfEmpty` defined in Task 2 Step 8, used in Task 2 Step 7 (same task, defined before the build step).

---

## Notes for the implementer

- If `pgx.RowToStructByName[storeItemRow]` errors on the `namespace TEXT[]` column, confirm pgx maps `[]string` ↔ `text[]` (it does by default via the pgtype registry); no custom codec needed.
- The `namespace[1:cardinality($1)] = $1` prefix match returns the exact prefix row too (a row whose namespace equals the prefix). That is intended for search. If a future requirement wants strict descendants only, add `AND cardinality(namespace) > cardinality($1)`.
- Do not open a PR or push until the user approves — this branch is local-only.
