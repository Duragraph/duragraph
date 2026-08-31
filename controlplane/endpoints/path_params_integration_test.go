package endpoints

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMalformedIdentifiersAreValidationErrors pins the API boundary for the
// thread state/checkpoint/history reads.
//
// These handlers used to interpolate raw path parameters straight into SQL and
// let Postgres do the validating, so every malformed identifier came back as a
// 500 carrying the raw driver text — e.g.
//
//	{"message":"ERROR: invalid input syntax for type bigint: \"not-a-bigint\" (SQLSTATE 22P02)"}
//
// which reports a client error as a server error AND leaks the column type.
//
// 422 is the contract-declared answer: these paths declare exactly "200" and
// "422" in the OpenAPI, so a 400 would be as undeclared as the 500 it replaces.
func TestMalformedIdentifiersAreValidationErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	var tid string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&tid); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, method, path, body string }{
		{"bad checkpoint id in path", "GET", "/api/v1/threads/" + tid + "/state/not-a-bigint", ""},
		{"checkpoint id out of int64 range", "GET", "/api/v1/threads/" + tid + "/state/99999999999999999999", ""},
		{"bad thread id on get_state", "GET", "/api/v1/threads/not-a-uuid/state", ""},
		{"bad thread id on get_history", "GET", "/api/v1/threads/not-a-uuid/history", ""},
		{"bad thread id on get_checkpoint_state", "GET", "/api/v1/threads/not-a-uuid/state/1", ""},
		{"bad checkpoint id in body", "POST", "/api/v1/threads/" + tid + "/state/checkpoint", `{"checkpoint":{"checkpoint_id":"abc"}}`},
		{"bad thread id on create_checkpoint", "POST", "/api/v1/threads/not-a-uuid/state/checkpoint", `{"checkpoint":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("want 422, got %d: %s", rec.Code, rec.Body.String())
			}
			// The response must not leak the schema or the driver.
			for _, leak := range []string{"SQLSTATE", "invalid input syntax", "bigint", "uuid:", "snapshots"} {
				if strings.Contains(rec.Body.String(), leak) {
					t.Errorf("response leaks internals (%q): %s", leak, rec.Body.String())
				}
			}
		})
	}
}

// TestWellFormedIdentifiersStillResolve is the companion guard: tightening the
// boundary must not turn a legitimate miss into a validation error. A
// well-formed id that simply matches nothing keeps its existing behavior.
func TestWellFormedIdentifiersStillResolve(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	var tid string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&tid); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"unknown but valid checkpoint id", "GET", "/api/v1/threads/" + tid + "/state/424242", "", http.StatusNotFound},
		{"unknown but valid thread id", "GET", "/api/v1/threads/11111111-1111-1111-1111-111111111111/state", "", http.StatusNotFound},
		{"history on a thread with no snapshots", "GET", "/api/v1/threads/" + tid + "/history", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			e.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("want %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}
