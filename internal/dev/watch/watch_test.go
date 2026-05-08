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
	stub := newStubSupervisor(t)
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

// TestWatcherDebounceTightBurst stresses the generation-token logic
// in scheduleReload: a tight burst of N events with tiny inter-arrival
// times exercises the path where time.AfterFunc may already be firing
// when the next Stop() arrives. The contract is: at most one
// dispatchReload per debounce window, regardless of burst size.
func TestWatcherDebounceTightBurst(t *testing.T) {
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
		DebounceWindow: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	stub := newStubSupervisor(t)
	w.supervisors[file] = stub.real

	// Fire a tight burst of events with no sleeps between them. The
	// first call schedules a timer; subsequent calls bump the
	// generation and Stop()/replace the timer. Some Stop() calls may
	// hit a timer that's already firing — the generation check must
	// reject those callbacks.
	const burst = 200
	for i := 0; i < burst; i++ {
		w.scheduleReload(file)
	}

	// Wait several debounce windows to let any in-flight callbacks
	// settle (the last-scheduled timer needs DebounceWindow to fire).
	time.Sleep(150 * time.Millisecond)

	got := stub.reloadCount()
	if got != 1 {
		t.Fatalf("expected exactly 1 reload after %d-event burst, got %d", burst, got)
	}
}

// stubSupervisor records Reload() invocations on a real
// *WorkerSupervisor without spawning subprocesses. We do this by
// constructing the supervisor and counting from a wrapper goroutine
// that drains its reload channel. The supervisor's Run() is never
// started, so no `uv` process exists.
//
// The drain goroutine selects on a done channel registered via
// t.Cleanup so the goroutine exits with the test instead of leaking
// for the lifetime of the test process. We can't close s.real.reload
// directly because Reload() does a non-blocking send and closing under
// it would panic.
type stubSupervisor struct {
	real *WorkerSupervisor
	mu   sync.Mutex
	n    int
	done chan struct{}
}

func newStubSupervisor(t *testing.T) *stubSupervisor {
	t.Helper()
	s := &stubSupervisor{
		real: newWorkerSupervisor(supervisorConfig{
			File:       "/tmp/stub.py",
			EnginePort: 9999,
		}),
		done: make(chan struct{}),
	}
	go func() {
		for {
			select {
			case <-s.done:
				return
			case <-s.real.reload:
				s.mu.Lock()
				s.n++
				s.mu.Unlock()
			}
		}
	}()
	t.Cleanup(func() { close(s.done) })
	return s
}

func (s *stubSupervisor) reloadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}
