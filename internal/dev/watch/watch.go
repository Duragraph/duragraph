// Package watch implements `duragraph dev`'s watch-mode worker
// supervision (binary-modes.yml § watch_mode). One Watcher manages a
// directory tree of Python graph files: it scans for files containing
// the @Graph( sentinel, spawns one subprocess supervisor per file, and
// reloads them via SIGTERM+respawn on file change.
//
// The package is deliberately small and Go-only — no Python interpreter,
// no Docker. Subprocesses are launched via `uv run --with-editable …`
// (or `--with duragraph` if no local SDK path is configured) and their
// stdout/stderr is line-prefixed with the source filename and forwarded
// to the engine's stdout/stderr so the operator sees a single unified
// log stream.
package watch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Options configures a Watcher. Construct with sane defaults via New;
// fields with zero values get spec-default fillers (200ms debounce,
// 10s SIGTERM grace, stdlib log.Default()).
type Options struct {
	// WatchDir is the directory to watch recursively for `.py` files.
	// If the directory does not exist, Run returns nil after a warning
	// — `duragraph dev`'s default of `./agents` should not error out
	// just because the operator hasn't created the dir yet.
	WatchDir string

	// EnginePort is injected into worker subprocesses as
	// DURAGRAPH_URL=http://localhost:<port>.
	EnginePort int

	// SDKPath is an optional local checkout of duragraph-python. When
	// set, workers run with `uv run --with-editable <sdkpath>`. When
	// empty, workers fall back to `uv run --with duragraph` (resolves
	// from PyPI). Wired from the DURAGRAPH_PYTHON_SDK_PATH env var by
	// New(); operators rarely set it directly.
	SDKPath string

	// Stdout / Stderr are the engine's writers. Each supervised worker
	// gets its own line-prefixed view of these (prefix = "[<basename>] ").
	// Default to os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// Logger receives watcher-level log output (spawned, reload, crash,
	// fsnotify error). Defaults to log.Default() — matches the rest of
	// duragraph's logger pattern (no central logger interface; serve.go
	// uses stdlib log directly).
	Logger *log.Logger

	// DebounceWindow is the quiet period (per file) before fsnotify
	// events trigger a reload. Default 200ms (per spec).
	DebounceWindow time.Duration

	// SIGTERMGrace is how long a worker has to exit after SIGTERM
	// before SIGKILL. Default 10s (per spec).
	SIGTERMGrace time.Duration

	// HealthURL is the engine's /health endpoint. The watcher polls
	// this until it returns 200 before scanning for graphs (otherwise
	// every spawned worker fails to register and crash-loops). Default
	// is http://localhost:<EnginePort>/health.
	HealthURL string

	// HealthTimeout caps how long Run waits for /health. Default 30s.
	HealthTimeout time.Duration
}

// Watcher is the public type returned by New. Run is the only exported
// method intended for external callers; Stop is implicit via context
// cancellation.
type Watcher struct {
	opts Options

	mu          sync.Mutex
	supervisors map[string]*WorkerSupervisor // file → supervisor
	debouncers  map[string]*debounceEntry    // file → pending reload state
	// closed flips true at the start of shutdown(). Guards against the
	// narrow race where a debounce timer fires between Stop()ing the
	// timer and clearing the map: the AfterFunc callback runs, enters
	// dispatchReload → spawnSupervisor, and inserts a brand-new
	// supervisor into the cleared map — its `uv` process would then
	// outlive the watcher with nothing to Stop() it. Checking closed
	// in spawn/dispatch turns those late callbacks into no-ops.
	closed bool
}

// debounceEntry tracks the pending reload state for one file. `generation`
// is incremented on every fsnotify event for the file; the AfterFunc
// callback captures the generation it was scheduled at and no-ops when
// it fires if a newer event has bumped the count. This is more robust
// than a Stop+Reset pattern because it tolerates the timer-already-firing
// race (Stop returns false but the callback proceeds): the generation
// check still rejects the stale callback.
type debounceEntry struct {
	timer      *time.Timer
	generation uint64
}

// New constructs a Watcher. Returns an error only on options that are
// invariably fatal (e.g. missing WatchDir + EnginePort = 0). Filesystem
// state and `uv` availability are deliberately checked at Run time so
// the operator sees errors in the engine's normal startup output.
func New(opts Options) (*Watcher, error) {
	if opts.EnginePort == 0 {
		return nil, errors.New("watch: EnginePort must be set")
	}
	if opts.WatchDir == "" {
		return nil, errors.New("watch: WatchDir must be set")
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.DebounceWindow == 0 {
		opts.DebounceWindow = 200 * time.Millisecond
	}
	if opts.SIGTERMGrace == 0 {
		opts.SIGTERMGrace = 10 * time.Second
	}
	if opts.HealthURL == "" {
		opts.HealthURL = fmt.Sprintf("http://localhost:%d/health", opts.EnginePort)
	}
	if opts.HealthTimeout == 0 {
		opts.HealthTimeout = 30 * time.Second
	}
	if opts.SDKPath == "" {
		// Allow override via env so operators developing the SDK can
		// point at their checkout without a CLI flag.
		opts.SDKPath = os.Getenv("DURAGRAPH_PYTHON_SDK_PATH")
	}
	return &Watcher{
		opts:        opts,
		supervisors: make(map[string]*WorkerSupervisor),
		debouncers:  make(map[string]*debounceEntry),
	}, nil
}

// Run blocks until ctx is cancelled (typically when the engine's main
// loop receives SIGINT/SIGTERM and we cancel its context). Returns nil
// on clean shutdown; any returned error is fatal (couldn't initialise
// fsnotify, couldn't reach engine /health within timeout, etc.).
//
// Phases:
//  1. Wait for the engine's /health to return 200 — otherwise every
//     worker we spawn fails to register against an unreachable port.
//  2. Resolve and stat the watch dir. Missing dir → log + return nil.
//  3. Initial scan: ScanGraphs the dir, spawn one supervisor per hit.
//  4. Start fsnotify, walk the tree to add every subdirectory (fsnotify
//     doesn't recurse — we manage that ourselves), and loop on events.
//  5. On ctx.Done: stop every supervisor, close fsnotify, return.
func (w *Watcher) Run(ctx context.Context) error {
	if err := w.waitForEngine(ctx); err != nil {
		return err
	}

	absDir, err := filepath.Abs(w.opts.WatchDir)
	if err != nil {
		return fmt.Errorf("resolve watch dir: %w", err)
	}
	info, statErr := os.Stat(absDir)
	if errors.Is(statErr, fs.ErrNotExist) {
		w.opts.Logger.Printf("watch: directory %s does not exist; skipping watch mode", absDir)
		<-ctx.Done()
		return nil
	}
	if statErr != nil {
		return fmt.Errorf("stat watch dir: %w", statErr)
	}
	if !info.IsDir() {
		return fmt.Errorf("watch: %s is not a directory", absDir)
	}

	fmt.Printf("🔍 Watch mode: scanning %s\n", absDir)

	// Initial scan.
	files, err := ScanGraphs(absDir)
	if err != nil {
		return fmt.Errorf("scan graphs: %w", err)
	}
	for _, f := range files {
		w.spawnSupervisor(f)
	}
	// From here on, every return path must tear down the supervisors we
	// just spawned — including the early-error returns from fsnotify
	// setup below. Defer guarantees that. shutdown() is idempotent
	// (closed=true is benign on repeat, the maps are reset on first
	// call, Stop/Wait on supervisors are idempotent), so it's safe to
	// run unconditionally.
	defer w.shutdown()

	// fsnotify setup. Has to recurse manually — Add() each subdir and
	// add new ones as they appear via Create events on directories.
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify init: %w", err)
	}
	defer fw.Close()
	if err := addRecursive(fw, absDir); err != nil {
		return fmt.Errorf("fsnotify add %s: %w", absDir, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			w.handleEvent(fw, ev)
		case ferr, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			w.opts.Logger.Printf("watch: fsnotify error: %v", ferr)
		}
	}
}

// waitForEngine polls /health until it returns 200, ctx is cancelled,
// or the timeout elapses. 200ms poll interval keeps the overhead
// negligible (~150 requests over 30s) without making the operator
// wait noticeably after the engine is up. Per-request timeout of 1s
// prevents a hung connect from soaking the whole budget.
func (w *Watcher) waitForEngine(ctx context.Context) error {
	deadline := time.Now().Add(w.opts.HealthTimeout)
	client := &http.Client{Timeout: time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.opts.HealthURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("watch: engine /health did not return 200 within %s (last url: %s)", w.opts.HealthTimeout, w.opts.HealthURL)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// handleEvent routes one fsnotify event. Directory creates are added
// to the watcher (so newly-mkdir'd subdirs aren't blind spots). File
// removes terminate any matching supervisor immediately. File writes
// and creates schedule a debounced reload — vim's atomic-rename pattern
// emits Rename+Create+Write for one save, and the 200ms quiet window
// collapses those into one reload action.
func (w *Watcher) handleEvent(fw *fsnotify.Watcher, ev fsnotify.Event) {
	// Directory create → recurse-add.
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if err := addRecursive(fw, ev.Name); err != nil {
				w.opts.Logger.Printf("watch: add subdir %s: %v", ev.Name, err)
			}
			return
		}
	}

	if filepath.Ext(ev.Name) != ".py" {
		return
	}

	// Remove / Rename of a tracked file → stop the supervisor.
	if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
		w.stopSupervisor(ev.Name)
		return
	}

	// Write / Create / Chmod → debounced reload.
	if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) || ev.Has(fsnotify.Chmod) {
		w.scheduleReload(ev.Name)
	}
}

// scheduleReload (re)starts the per-file debounce timer. After the
// quiet window elapses, dispatchReload re-checks the @Graph( sentinel
// (so a file the user emptied isn't supervised) and either reloads,
// spawns, or stops the supervisor accordingly.
//
// Per-file generation tokens guard against the time.AfterFunc-already-
// firing race: each event bumps entry.generation; the callback captures
// the generation it was scheduled at, and on fire it re-checks under
// the mutex. If a newer event arrived (generation advanced), the
// callback no-ops and the newer event's callback will dispatch.
func (w *Watcher) scheduleReload(file string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry, ok := w.debouncers[file]
	if !ok {
		entry = &debounceEntry{}
		w.debouncers[file] = entry
	}
	entry.generation++
	expected := entry.generation

	if entry.timer != nil {
		// Best-effort cancel; if the timer already fired the callback
		// will see a mismatched generation and exit cleanly.
		entry.timer.Stop()
	}
	entry.timer = time.AfterFunc(w.opts.DebounceWindow, func() {
		w.mu.Lock()
		e, ok := w.debouncers[file]
		if !ok || e.generation != expected {
			// Superseded by a newer event (or already cleared by
			// shutdown). The newer event's callback will dispatch.
			w.mu.Unlock()
			return
		}
		// We won the race — claim this dispatch by removing the entry.
		delete(w.debouncers, file)
		w.mu.Unlock()
		w.dispatchReload(file)
	})
}

// dispatchReload is the post-debounce action: re-check sentinel, then
// reload / spawn / stop. Held outside the lock so we don't call into
// supervisor channels while holding the watcher mutex.
func (w *Watcher) dispatchReload(file string) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	sup, exists := w.supervisors[file]
	w.mu.Unlock()

	hasGraph, err := HasGraphDecorator(file)
	if err != nil {
		// File deleted between debounce schedule and fire, or unreadable.
		if exists {
			w.stopSupervisor(file)
		}
		return
	}
	if !hasGraph {
		if exists {
			w.stopSupervisor(file)
		}
		return
	}
	if exists {
		sup.Reload()
		return
	}
	w.spawnSupervisor(file)
}

// spawnSupervisor wires up a new supervisor for a file and starts its
// Run goroutine. Caller must NOT already hold the lock for this file.
// No-op if the watcher is shutting down (see Watcher.closed).
func (w *Watcher) spawnSupervisor(file string) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	if _, exists := w.supervisors[file]; exists {
		w.mu.Unlock()
		return
	}
	sup := newWorkerSupervisor(supervisorConfig{
		File:         file,
		EnginePort:   w.opts.EnginePort,
		SDKPath:      w.opts.SDKPath,
		Stdout:       w.opts.Stdout,
		Stderr:       w.opts.Stderr,
		Logger:       w.opts.Logger,
		SIGTERMGrace: w.opts.SIGTERMGrace,
	})
	w.supervisors[file] = sup
	w.mu.Unlock()
	go sup.Run()
}

// stopSupervisor removes the supervisor for file (if any), signals it
// to stop, and waits for it to drain. Synchronous so a quick
// remove-then-create sequence doesn't race the new spawn against the
// old kill.
func (w *Watcher) stopSupervisor(file string) {
	w.mu.Lock()
	sup, ok := w.supervisors[file]
	if ok {
		delete(w.supervisors, file)
	}
	w.mu.Unlock()
	if !ok {
		return
	}
	sup.Stop()
	sup.Wait()
}

// shutdown stops every supervisor in parallel and waits for all to
// drain. Called from Run when ctx is cancelled.
func (w *Watcher) shutdown() {
	w.mu.Lock()
	w.closed = true
	sups := make([]*WorkerSupervisor, 0, len(w.supervisors))
	for _, s := range w.supervisors {
		sups = append(sups, s)
	}
	w.supervisors = make(map[string]*WorkerSupervisor)
	// Cancel any pending debounce timers so AfterFunc callbacks don't
	// fire after Run returns and try to spawn supervisors into a
	// shutting-down watcher. Even if a timer has already fired and the
	// callback is in flight, the dispatchReload path checks w.closed
	// and the schedule-side generation check rejects on missing entry.
	for _, e := range w.debouncers {
		if e.timer != nil {
			e.timer.Stop()
		}
	}
	w.debouncers = make(map[string]*debounceEntry)
	w.mu.Unlock()

	var wg sync.WaitGroup
	for _, s := range sups {
		s.Stop()
	}
	for _, s := range sups {
		wg.Add(1)
		go func(s *WorkerSupervisor) { defer wg.Done(); s.Wait() }(s)
	}
	wg.Wait()
}

// addRecursive walks root and Add()s every directory to fw. fsnotify
// doesn't recurse on its own; without this, edits to files in
// subdirectories silently never trigger reloads. Hidden / __pycache__
// dirs are skipped to avoid burning watcher slots on noise (matches
// scanner skip list).
func addRecursive(fw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (strings.HasPrefix(name, ".") || name == "__pycache__" || name == "node_modules") {
			return fs.SkipDir
		}
		if err := fw.Add(path); err != nil {
			return fmt.Errorf("fsnotify add %s: %w", path, err)
		}
		return nil
	})
}
