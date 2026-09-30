package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/duragraph/duragraph/go-sdk/graph"
)

type echoNode struct{}

func (echoNode) Execute(_ context.Context, state map[string]any) (map[string]any, error) {
	state["done"] = true
	return state, nil
}

type failNode struct{}

func (failNode) Execute(_ context.Context, state map[string]any) (map[string]any, error) {
	return state, errors.New("node exploded")
}
func testGraph() *graph.Graph[map[string]any] {
	g := graph.New[map[string]any]("test-graph")
	g.AddNode("echo", echoNode{})
	g.SetEntrypoint("echo")
	return g
}

// A wire fixture intentionally rejects all routes except the implemented
// controlplane/endpoints/workers_gen.go protocol; old /poll calls fail tests.
type wire struct {
	mu         sync.Mutex
	paths      []string
	events     []map[string]any
	checkpoint json.RawMessage
	version    int
	epoch      int
}

func (f *wire) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if r.Method == http.MethodPost {
			if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
				t.Errorf("decode %s: %v", r.URL.Path, e)
				w.WriteHeader(400)
				return
			}
		}
		switch {
		case r.URL.Path == "/api/v1/workers/register":
			if _, e := uuid.Parse(body["worker_id"].(string)); e != nil {
				t.Errorf("worker id is not UUID: %v", e)
			}
			if body["capacity"] != float64(1) || body["graphs"].([]any)[0] != "test-graph" {
				t.Errorf("register payload: %v", body)
			}
			def := body["graph_definitions"].([]any)[0].(map[string]any)
			if def["name"] != "test-graph" || def["nodes"] == nil {
				t.Errorf("graph registration: %v", def)
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"worker_id": body["worker_id"], "status": "online"}); err != nil {
				t.Errorf("encode register: %v", err)
			}
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			if body["status"] != "online" {
				t.Errorf("heartbeat status: %v", body)
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"commands": []string{}}); err != nil {
				t.Errorf("encode heartbeat: %v", err)
			}
		case strings.HasSuffix(r.URL.Path, "/runs/claim"):
			if body["max_runs"] != float64(1) {
				t.Errorf("claim payload: %v", body)
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"runs": []any{}}); err != nil {
				t.Errorf("encode claim: %v", err)
			}
		case strings.HasSuffix(r.URL.Path, "/events"):
			events := body["events"].([]any)
			for _, v := range events {
				ev := v.(map[string]any)
				f.events = append(f.events, ev)
				if ev["type"] == "run.started" {
					f.epoch++
					if err := json.NewEncoder(w).Encode(map[string]any{"lease_epoch": f.epoch}); err != nil {
						t.Errorf("encode epoch: %v", err)
					}
					return
				}
				if ev["lease_epoch"] != float64(f.epoch) {
					t.Errorf("unfenced event: %v", ev)
				}
			}
			w.WriteHeader(200)
		case strings.HasSuffix(r.URL.Path, "/graph"):
			if err := json.NewEncoder(w).Encode(map[string]any{"nodes": []any{map[string]string{"id": "echo", "type": "function"}}, "edges": []any{}, "config": map[string]any{}}); err != nil {
				t.Errorf("encode graph: %v", err)
			}
		case strings.HasSuffix(r.URL.Path, "/checkpoints/latest"):
			if f.checkpoint == nil {
				w.WriteHeader(404)
				return
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"checkpoint_id": 1, "version": f.version, "state": f.checkpoint}); err != nil {
				t.Errorf("encode checkpoint: %v", err)
			}
		case strings.HasSuffix(r.URL.Path, "/checkpoints"):
			if body["lease_epoch"] != float64(f.epoch) {
				t.Errorf("unfenced checkpoint: %v", body)
			}
			var err error
			f.checkpoint, err = json.Marshal(body["state"])
			if err != nil {
				t.Errorf("marshal checkpoint: %v", err)
			}
			f.version = int(body["version"].(float64))
			if err := json.NewEncoder(w).Encode(map[string]int{"checkpoint_id": 1}); err != nil {
				t.Errorf("encode checkpoint write: %v", err)
			}
		case strings.HasSuffix(r.URL.Path, "/deregister"):
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected protocol route: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
}

func TestRegisterAndClaim(t *testing.T) {
	f := new(wire)
	s := httptest.NewServer(f.handler(t))
	defer s.Close()
	w := New(testGraph(), WithControlPlane(s.URL), WithAPIKey("secret"))
	if err := w.register(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if err := w.claimLoop(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.Contains(strings.Join(f.paths, " "), "/runs/claim") {
		t.Fatal("no claim request")
	}
}

func TestRunWireAndResume(t *testing.T) {
	tests := []struct {
		name    string
		claimed bool
		resumed bool
	}{{"claimed", true, false}, {"push", false, false}, {"resume from finished checkpoint", false, true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := new(wire)
			f.epoch = 7
			if tt.resumed {
				f.version = 1
				f.checkpoint = json.RawMessage(`{"state":{"done":true},"next":""}`)
			}
			s := httptest.NewServer(f.handler(t))
			defer s.Close()
			w := New(testGraph(), WithControlPlane(s.URL))
			w.workerID = uuid.NewString()
			task := RunTask{RunID: uuid.NewString(), ThreadID: uuid.NewString(), GraphID: "test-graph", Input: json.RawMessage(`{"message":"hi"}`), LeaseEpoch: 7}
			if err := w.executeRun(context.Background(), task, tt.claimed); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			want := []string{"execution.node_started", "execution.node_completed", "run.completed"}
			if !tt.claimed {
				want = append([]string{"run.started"}, want...)
			}
			if tt.resumed {
				want = []string{"run.started", "run.completed"}
			}
			if len(f.events) != len(want) {
				t.Fatalf("events = %v, want %v", f.events, want)
			}
			for i, event := range f.events {
				if event["type"] != want[i] {
					t.Errorf("event %d = %v, want %s", i, event, want[i])
				}
			}
			if !tt.resumed && f.version != 1 {
				t.Errorf("checkpoint version = %d", f.version)
			}
			if f.events[len(f.events)-1]["output"] == nil {
				t.Error("run.completed omitted output")
			}
		})
	}
}

func TestStaleLeaseAndGraphMismatch(t *testing.T) {
	for _, tt := range []struct {
		name     string
		code     int
		mismatch bool
	}{{"stale", 409, false}, {"graph mismatch", 200, true}} {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.mismatch {
					if err := json.NewEncoder(w).Encode(map[string]any{"nodes": []any{map[string]string{"id": "other"}}, "edges": []any{}}); err != nil {
						t.Errorf("encode mismatch: %v", err)
					}
				} else {
					w.WriteHeader(tt.code)
				}
			}))
			defer s.Close()
			wk := New(testGraph(), WithControlPlane(s.URL))
			task := RunTask{RunID: uuid.NewString(), ThreadID: uuid.NewString(), LeaseEpoch: 1}
			err := wk.executeRun(context.Background(), task, true)
			if tt.mismatch {
				if err == nil || !strings.Contains(err.Error(), "graph differs") {
					t.Fatalf("want graph mismatch, got %v", err)
				}
			} else if !errors.Is(err, ErrStaleLease) {
				t.Fatalf("want stale lease, got %v", err)
			}
		})
	}
}

func TestNodeFailureIsRecordedBeforeTerminal(t *testing.T) {
	f := new(wire)
	f.epoch = 2
	s := httptest.NewServer(f.handler(t))
	defer s.Close()
	g := graph.New[map[string]any]("test-graph")
	g.AddNode("echo", failNode{})
	g.SetEntrypoint("echo")
	w := New(g, WithControlPlane(s.URL))
	task := RunTask{RunID: uuid.NewString(), ThreadID: uuid.NewString(), LeaseEpoch: 2}
	if err := w.executeRun(context.Background(), task, true); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var got []string
	for _, ev := range f.events {
		got = append(got, ev["type"].(string))
	}
	if strings.Join(got, ",") != "execution.node_started,execution.node_failed,run.failed" {
		t.Fatalf("events: %v", got)
	}
	if f.events[2]["error"] != "node echo: node exploded" {
		t.Errorf("failure payload: %v", f.events[2])
	}
}

func TestResumeCommandFailsClosed(t *testing.T) {
	f := new(wire)
	f.epoch = 2
	s := httptest.NewServer(f.handler(t))
	defer s.Close()
	w := New(testGraph(), WithControlPlane(s.URL))
	task := RunTask{RunID: uuid.NewString(), ThreadID: uuid.NewString(), LeaseEpoch: 2, Resume: json.RawMessage(`{"resume":"yes"}`)}
	err := w.executeRun(context.Background(), task, true)
	if err == nil || !strings.Contains(err.Error(), "HITL") {
		t.Fatalf("want explicit HITL failure, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.events {
		if e["type"] == "run.completed" {
			t.Fatal("resumed run reported completed")
		}
	}
}

func TestStartRequiresConfiguration(t *testing.T) {
	if err := New(testGraph()).Start(context.Background()); err == nil {
		t.Fatal("missing control plane")
	}
}
