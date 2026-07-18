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

	// decoy: seed an item under a different top-level namespace. Proves the
	// array-prefix match (namespace[1:cardinality($1)] = $1) is exact-array,
	// not a substring/partial match.
	_ = doJSON(t, e, http.MethodPut, "/api/v1/store/items",
		`{"namespace":["orders","7"],"key":"summary","value":{"status":"shipped"}}`)

	// search: prefix ["users"] must exclude the "orders" decoy entirely.
	rec = doJSON(t, e, http.MethodPost, "/api/v1/store/items/search", `{"namespace_prefix":["users"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("search prefix users: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sr); err != nil {
		t.Fatalf("search prefix users decode: %v", err)
	}
	if len(sr.Items) != 2 {
		t.Errorf("search prefix users: want 2 items, got %d: %+v", len(sr.Items), sr.Items)
	}
	for _, it := range sr.Items {
		if len(it.Namespace) == 0 || it.Namespace[0] != "users" {
			t.Errorf("search prefix users: decoy leaked into results: %+v", it)
		}
	}

	// search: no namespace_prefix field -> exercises the SQL nil-guard
	// ($1::text[] IS NULL OR ...) and must match across ALL namespaces.
	rec = doJSON(t, e, http.MethodPost, "/api/v1/store/items/search", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("search no prefix: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sr); err != nil {
		t.Fatalf("search no prefix decode: %v", err)
	}
	var sawUsers, sawOrders bool
	for _, it := range sr.Items {
		if len(it.Namespace) == 0 {
			continue
		}
		switch it.Namespace[0] {
		case "users":
			sawUsers = true
		case "orders":
			sawOrders = true
		}
	}
	if !sawUsers || !sawOrders {
		t.Errorf("search no prefix: want items from both users and orders namespaces, got %+v", sr.Items)
	}

	// list_namespaces: no prefix field -> same nil-guard, must return
	// distinct namespaces across ALL items, including the decoy's.
	rec = doJSON(t, e, http.MethodPost, "/api/v1/store/namespaces", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("namespaces no prefix: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var nsAll ListNamespaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &nsAll); err != nil {
		t.Fatalf("namespaces no prefix decode: %v", err)
	}
	if len(nsAll) != 3 {
		t.Errorf("namespaces no prefix: want 3 distinct namespaces, got %d: %v", len(nsAll), nsAll)
	}
	var hasOrders bool
	for _, n := range nsAll {
		if len(n) > 0 && n[0] == "orders" {
			hasOrders = true
		}
	}
	if !hasOrders {
		t.Errorf("namespaces no prefix: decoy namespace missing from result: %v", nsAll)
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
