package watch

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// supervisorConfig is the per-file slice of Watcher options the
// supervisor needs. Pulled out as its own struct to keep WorkerSupervisor
// constructable from tests without dragging in fsnotify or logger
// plumbing.
type supervisorConfig struct {
	File         string        // absolute path to the .py file
	EnginePort   int           // for DURAGRAPH_URL env injection
	SDKPath      string        // optional path to local duragraph-python
	Stdout       io.Writer     // engine's stdout (will be prefix-wrapped)
	Stderr       io.Writer     // engine's stderr (will be prefix-wrapped)
	Logger       *log.Logger   // for supervisor-level messages, not worker output
	SIGTERMGrace time.Duration // SIGTERM → SIGKILL grace
}

// reloadSignal is what the watcher sends to a supervisor when the file
// changes. The supervisor stops the current worker (graceful kill,
// then respawn) and resets its crash backoff.
type reloadSignal struct{}

// WorkerSupervisor owns the lifecycle of one worker subprocess for one
// .py file. A single goroutine (run) loops over: spawn → wait for exit
// → decide next action (reload? backoff? exit?). Reload signals and
// shutdown are coordinated via channels; the supervisor never holds a
// lock across a subprocess call.
//
// Crash backoff schedule: 1s, 2s, 4s, 8s, 16s, 32s, 60s (cap). On any
// reload signal the backoff is reset to zero so a fix-and-save sees
// the worker come back instantly even after it had been crash-looping.
type WorkerSupervisor struct {
	cfg supervisorConfig

	// reload is signalled by the Watcher when the file mtime changes
	// (after debounce). Buffered size 1 so a Reload() call from the
	// fsnotify callback never blocks the event loop.
	reload chan reloadSignal
	// stop closes when Run should terminate. The supervisor reacts by
	// killing any live child and returning.
	stop chan struct{}
	// done closes after Run returns; lets Watcher.Run wait for clean
	// shutdown.
	done chan struct{}

	// stdoutPW / stderrPW are the line-buffered prefix writers. We hold
	// references so we can Flush() them on subprocess exit.
	stdoutPW *prefixWriter
	stderrPW *prefixWriter
}

// newWorkerSupervisor wires the channels and prefix writers but does
// not spawn anything. Call Run in a goroutine to start work.
func newWorkerSupervisor(cfg supervisorConfig) *WorkerSupervisor {
	prefix := "[" + filepath.Base(cfg.File) + "] "
	if cfg.SIGTERMGrace == 0 {
		cfg.SIGTERMGrace = 10 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	return &WorkerSupervisor{
		cfg:      cfg,
		reload:   make(chan reloadSignal, 1),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		stdoutPW: newPrefixWriter(prefix, cfg.Stdout),
		stderrPW: newPrefixWriter(prefix, cfg.Stderr),
	}
}

// Reload signals the supervisor to kill its current worker and respawn.
// Non-blocking: if a reload is already pending, this is a no-op (the
// pending signal already represents "one reload owed").
func (s *WorkerSupervisor) Reload() {
	select {
	case s.reload <- reloadSignal{}:
	default:
	}
}

// Stop signals the supervisor to wind down. Stop is idempotent —
// closing an already-closed channel would panic, so we guard with a
// select.
func (s *WorkerSupervisor) Stop() {
	select {
	case <-s.stop:
		// Already stopped.
	default:
		close(s.stop)
	}
}

// Wait blocks until Run returns. Safe to call after Stop; intended for
// the watcher's shutdown path.
func (s *WorkerSupervisor) Wait() { <-s.done }

// Run is the supervisor's main loop. It spawns the worker, waits for
// exit, then decides:
//   - stop signalled  → return
//   - reload signalled → kill (if alive), reset backoff, respawn now
//   - clean exit (rc==0)  → back off and respawn (a worker that exits
//     cleanly without our help usually means a misconfigured `if
//     __name__ == "__main__":` block; respawning instantly would just
//     spam the same exit, so we apply the same backoff schedule as for
//     crashes)
//   - non-zero exit  → backoff, then respawn
//
// One-goroutine design: this loop is the only place that touches
// s.cmd / s.cmd.Wait, so there's no concurrent access to *exec.Cmd.
func (s *WorkerSupervisor) Run() {
	defer close(s.done)
	defer s.stdoutPW.Flush()
	defer s.stderrPW.Flush()

	backoff := backoffSequence{}
	for {
		// Spawn a fresh subprocess. On success this returns a started
		// *exec.Cmd plus a channel that will receive its exit error.
		cmd, exitCh, err := s.spawn()
		if err != nil {
			// Spawn-time failures are usually missing `uv` or a bad
			// SDK path. Don't loop forever on those — log and back off
			// like a crash so the operator sees the message and the
			// supervisor stays responsive to file edits.
			s.cfg.Logger.Printf("watch: spawn failed for %s: %v", s.cfg.File, err)
			if !s.sleepOrEvent(backoff.next()) {
				return
			}
			continue
		}
		s.cfg.Logger.Printf("watch: spawned worker for %s (pid %d)", filepath.Base(s.cfg.File), cmd.Process.Pid)

		// Wait for either: subprocess exits on its own, reload, or stop.
		select {
		case err := <-exitCh:
			// Subprocess exited without our help. Treat as crash if
			// rc != 0; back off and respawn.
			rc := exitCode(err)
			if rc == 0 {
				s.cfg.Logger.Printf("watch: worker for %s exited cleanly; restarting after backoff", filepath.Base(s.cfg.File))
			} else {
				s.cfg.Logger.Printf("watch: worker for %s exited with code %d; restarting after backoff", filepath.Base(s.cfg.File), rc)
			}
			s.stdoutPW.Flush()
			s.stderrPW.Flush()
			if !s.sleepOrEvent(backoff.next()) {
				return
			}
		case <-s.reload:
			// File change: kill, drain exit, reset backoff, respawn now.
			s.cfg.Logger.Printf("watch: reload requested for %s", filepath.Base(s.cfg.File))
			s.terminate(cmd, exitCh)
			s.stdoutPW.Flush()
			s.stderrPW.Flush()
			backoff.reset()
			// Spec calls for waiting for the worker_id to deregister.
			// Doing that properly requires plumbing the engine API +
			// extracting the worker_id from the Python SDK output.
			// For Phase 5 we approximate with a 1s settle delay so the
			// new worker registers cleanly without a duplicate-id
			// collision against the old one. Operators will see a
			// short blank gap on reload, which is acceptable.
			time.Sleep(1 * time.Second)
		case <-s.stop:
			s.terminate(cmd, exitCh)
			return
		}
	}
}

// spawn starts a new subprocess. Returns the started cmd, a channel
// that will receive its eventual exit error (nil for rc=0, *ExitError
// otherwise), and a start-time error if the binary couldn't be exec'd.
//
// Process group: we set Setpgid so we can signal the entire group
// (uv → python → user code). Without this, signalling cmd.Process
// kills only `uv`, which leaves orphan python processes behind.
func (s *WorkerSupervisor) spawn() (*exec.Cmd, <-chan error, error) {
	args, err := buildSpawnArgs(s.cfg.SDKPath, s.cfg.File)
	if err != nil {
		return nil, nil, err
	}
	// NOTE: do NOT use exec.CommandContext here. CommandContext SIGKILLs
	// on context cancel, defeating our 10s SIGTERM grace. We do our own
	// signal sequencing in terminate().
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("DURAGRAPH_URL=http://localhost:%d", s.cfg.EnginePort),
	)
	cmd.Stdout = s.stdoutPW
	cmd.Stderr = s.stderrPW
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	exitCh := make(chan error, 1)
	go func() { exitCh <- cmd.Wait() }()
	return cmd, exitCh, nil
}

// terminate sends SIGTERM to the worker's process group, waits up to
// SIGTERMGrace for the exitCh to fire, then SIGKILLs if needed. Idempotent
// w.r.t. an already-dead process — if the kill returns ESRCH, fine.
func (s *WorkerSupervisor) terminate(cmd *exec.Cmd, exitCh <-chan error) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid := cmd.Process.Pid
	// Negative pid = signal the whole process group (uv + child python).
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		s.cfg.Logger.Printf("watch: SIGTERM %s: %v", filepath.Base(s.cfg.File), err)
	}
	select {
	case <-exitCh:
		return
	case <-time.After(s.cfg.SIGTERMGrace):
		s.cfg.Logger.Printf("watch: %s did not exit within %s; sending SIGKILL", filepath.Base(s.cfg.File), s.cfg.SIGTERMGrace)
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			s.cfg.Logger.Printf("watch: SIGKILL %s: %v", filepath.Base(s.cfg.File), err)
		}
		<-exitCh
	}
}

// sleepOrEvent sleeps for d unless a stop or reload arrives first.
// Returns false if stop fired (caller should return), true otherwise.
// A reload during the sleep is consumed by the channel here and would
// otherwise be lost — to preserve it, we re-signal Reload before
// returning so the next iteration sees it and respawns immediately.
func (s *WorkerSupervisor) sleepOrEvent(d time.Duration) bool {
	if d <= 0 {
		select {
		case <-s.stop:
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-s.reload:
		// Re-queue the reload so the loop body sees it.
		s.Reload()
		return true
	case <-s.stop:
		return false
	}
}

// buildSpawnArgs returns the argv for the subprocess. If sdkPath is
// non-empty, --with-editable points at it; otherwise we fall back to
// `--with duragraph` and let uv resolve from PyPI. uv's absence is
// detected here so the operator sees a clear, actionable message
// instead of a cryptic exec error.
func buildSpawnArgs(sdkPath, file string) ([]string, error) {
	if _, err := exec.LookPath("uv"); err != nil {
		return nil, fmt.Errorf("watch mode requires `uv` (https://docs.astral.sh/uv/). Install it or run without --watch")
	}
	if sdkPath != "" {
		return []string{"uv", "run", "--with-editable", sdkPath, "python", file}, nil
	}
	return []string{"uv", "run", "--with", "duragraph", "python", file}, nil
}

// exitCode extracts the integer exit code from a cmd.Wait() error.
// Returns 0 for nil (clean exit), the rc for *exec.ExitError, and -1
// for other errors (e.g. I/O on the pipe — treat as crash).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// backoffSequence implements the spec's 1s, 2s, 4s, 8s, 16s, 32s, 60s
// (cap) crash-backoff schedule. reset() drops the index so the next
// next() returns 1s — used after a successful reload signal.
type backoffSequence struct {
	idx int
}

var backoffSteps = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	16 * time.Second,
	32 * time.Second,
	60 * time.Second,
}

func (b *backoffSequence) next() time.Duration {
	d := backoffSteps[b.idx]
	if b.idx < len(backoffSteps)-1 {
		b.idx++
	}
	return d
}

func (b *backoffSequence) reset() { b.idx = 0 }
