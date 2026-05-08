package watch

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewValidation(t *testing.T) {
	if _, err := New(Options{WatchDir: "/tmp"}); err == nil {
		t.Fatal("expected error: missing EnginePort")
	}
	if _, err := New(Options{EnginePort: 8081}); err == nil {
		t.Fatal("expected error: missing WatchDir")
	}
}

func TestWatcherMissingDirReturnsCleanly(t *testing.T) {
	// /health stub that immediately returns 200 so waitForEngine passes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := New(Options{
		WatchDir:      filepath.Join(t.TempDir(), "does-not-exist"),
		EnginePort:    9999,
		HealthURL:     srv.URL,
		HealthTimeout: 2 * time.Second,
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		Logger:        log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run on missing dir: %v", err)
	}
}

func TestWatcherHealthTimeout(t *testing.T) {
	// Bind a closed server so the watcher can't reach /health.
	w, err := New(Options{
		WatchDir:      t.TempDir(),
		EnginePort:    1, // unprivileged; never bound
		HealthURL:     "http://127.0.0.1:1/health",
		HealthTimeout: 100 * time.Millisecond,
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		Logger:        log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = w.Run(context.Background())
	if err == nil {
		t.Fatal("expected health timeout error")
	}
}

func TestWatcherDebouncesEvents(t *testing.T) {
	// We exercise scheduleReload + dispatchReload directly rather than
	// stand up a real watcher loop — the dispatch path is what
	// determines reload semantics. We arrange dispatchReload to spawn
	// a recorded supervisor stand-in via spawnSupervisor; since
	// spawnSupervisor calls newWorkerSupervisor which kicks off Run(),
	// we'd actually try to exec uv. So instead we pre-populate the
	// supervisors map with a stub that records Reload() calls.

	dir := t.TempDir()
	file := filepath.Join(dir, "graph.py")
	if err := os.WriteFile(file, []byte("@Graph()\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := New(Options{
		WatchDir:       dir,
		EnginePort:     9999,
		HealthURL:      srv.URL,
		HealthTimeout:  2 * time.Second,
		Stdout:         io.Discard,
		Stderr:         io.Discard,
		Logger:         log.New(io.Discard, "", 0),
		DebounceWindow: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Inject a stub supervisor into the map so dispatchReload calls
	// Reload() on it instead of trying to spawn `uv`.
	stub := newStubSupervisor()
	w.supervisors[file] = stub.real

	// Three rapid events should collapse to one Reload call.
	for i := 0; i < 3; i++ {
		w.scheduleReload(file)
		time.Sleep(10 * time.Millisecond) // shorter than debounce
	}

	// Wait past the debounce window for the timer to fire.
	time.Sleep(120 * time.Millisecond)

	got := stub.reloadCount()
	if got != 1 {
		t.Fatalf("expected 1 reload after debounce, got %d", got)
	}

	// A subsequent event after the window has lapsed → another reload.
	w.scheduleReload(file)
	time.Sleep(120 * time.Millisecond)
	got = stub.reloadCount()
	if got != 2 {
		t.Fatalf("expected 2 reloads after second debounce, got %d", got)
	}
}

// stubSupervisor records Reload() invocations on a real
// *WorkerSupervisor without spawning subprocesses. We do this by
// constructing the supervisor and counting from a wrapper goroutine
// that drains its reload channel. The supervisor's Run() is never
// started, so no `uv` process exists.
type stubSupervisor struct {
	real *WorkerSupervisor
	mu   sync.Mutex
	n    int
}

func newStubSupervisor() *stubSupervisor {
	s := &stubSupervisor{
		real: newWorkerSupervisor(supervisorConfig{
			File:       "/tmp/stub.py",
			EnginePort: 9999,
		}),
	}
	go func() {
		for range s.real.reload {
			s.mu.Lock()
			s.n++
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *stubSupervisor) reloadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}
