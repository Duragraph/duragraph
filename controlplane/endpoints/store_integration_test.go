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
