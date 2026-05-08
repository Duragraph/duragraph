package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetRun_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runs/abc" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"run_id":"abc","status":"completed"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	body, err := c.GetRun(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if !strings.Contains(string(body), `"run_id":"abc"`) {
		t.Fatalf("GetRun body: %s", body)
	}
}

func TestGetRun_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not_found"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.GetRun(context.Background(), "missing")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("GetRun(missing): expected ErrRunNotFound, got %v", err)
	}
}

func TestCreateRun_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/runs" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["assistant_id"] != "asst_1" {
			t.Errorf("unexpected body: %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"run_id":"r1","thread_id":"t1","assistant_id":"asst_1","status":"queued"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	resp, err := c.CreateRun(context.Background(), map[string]any{
		"assistant_id": "asst_1",
		"input":        map[string]any{"x": 1},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if resp.RunID != "r1" || resp.ThreadID != "t1" || resp.Status != "queued" {
		t.Fatalf("CreateRun response: %+v", resp)
	}
}

func TestCreateRun_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"boom"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.CreateRun(context.Background(), map[string]any{"assistant_id": "x"})
	if err == nil {
		t.Fatal("CreateRun: expected error on 500")
	}
}

func TestWaitForRun_PollsUntilTerminal(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			fmt.Fprint(w, `{"run_id":"r1","status":"in_progress"}`)
			return
		}
		fmt.Fprint(w, `{"run_id":"r1","status":"completed","output":{"ok":true}}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	body, err := c.WaitForRun(context.Background(), "r1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForRun: %v", err)
	}
	if !strings.Contains(string(body), `"status":"completed"`) {
		t.Fatalf("WaitForRun: expected completed body, got %s", body)
	}
	if atomic.LoadInt32(&hits) < 3 {
		t.Fatalf("WaitForRun: expected at least 3 polls, got %d", hits)
	}
}

func TestWaitForRun_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"run_id":"r1","status":"in_progress"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := c.WaitForRun(ctx, "r1", 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForRun: expected DeadlineExceeded, got %v", err)
	}
}

func TestStreamRuns_ParsesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/threads/t1/stream" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "event: run.started\ndata: {\"run_id\":\"r1\"}\n\n")
		fmt.Fprint(w, ": keepalive\n\n")
		fmt.Fprint(w, "event: run.completed\ndata: {\"run_id\":\"r1\",\"status\":\"ok\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var got []SSEEvent
	err := c.StreamRuns(ctx, "t1", func(ev SSEEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamRuns: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("StreamRuns: expected 2 events, got %d (%+v)", len(got), got)
	}
	if got[0].Type != "run.started" || !strings.Contains(string(got[0].Data), `"run_id":"r1"`) {
		t.Fatalf("StreamRuns event[0]: %+v", got[0])
	}
	if got[1].Type != "run.completed" {
		t.Fatalf("StreamRuns event[1]: %+v", got[1])
	}
}

func TestStreamRuns_RequiresThreadID(t *testing.T) {
	c := NewClient("http://127.0.0.1:1")
	err := c.StreamRuns(context.Background(), "", func(SSEEvent) error { return nil })
	if err == nil {
		t.Fatal("StreamRuns: expected error for empty threadID")
	}
}

func TestParseSSE_MultiLineData(t *testing.T) {
	// W3C SSE: multiple data: lines join with '\n'. The engine never
	// produces this shape but the parser should handle it.
	in := strings.NewReader("event: x\ndata: line1\ndata: line2\n\n")
	var got []SSEEvent
	err := parseSSE(in, func(ev SSEEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSE: %v", err)
	}
	if len(got) != 1 || string(got[0].Data) != "line1\nline2" {
		t.Fatalf("parseSSE multi-line: %+v", got)
	}
}
