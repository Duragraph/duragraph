package watch

import (
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestBackoffSequence(t *testing.T) {
	b := backoffSequence{}
	want := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		60 * time.Second,
		60 * time.Second, // capped
		60 * time.Second, // still capped
	}
	for i, w := range want {
		if got := b.next(); got != w {
			t.Fatalf("step %d: got %s want %s", i, got, w)
		}
	}
	b.reset()
	if got := b.next(); got != 1*time.Second {
		t.Fatalf("after reset: got %s want 1s", got)
	}
}

func TestBuildSpawnArgsRequiresUv(t *testing.T) {
	// We can't reliably test the success path without making `uv` a
	// test runner dependency. The failure path is the contract that
	// matters for operator UX: we must return a clear error if `uv`
	// isn't on PATH. Force PATH to a directory that can't contain uv.
	t.Setenv("PATH", t.TempDir())
	_, err := buildSpawnArgs("", "/tmp/file.py")
	if err == nil {
		t.Fatal("expected error when uv missing")
	}
	if got := err.Error(); !contains(got, "uv") {
		t.Fatalf("error must mention uv: %q", got)
	}
}

func TestExitCode(t *testing.T) {
	if got := exitCode(nil); got != 0 {
		t.Fatalf("nil: got %d want 0", got)
	}
	if got := exitCode(errors.New("io broken")); got != -1 {
		t.Fatalf("non-exit: got %d want -1", got)
	}
	// Synthesize a real *exec.ExitError by running a command we know
	// will exit non-zero. `false` exists on every Linux test runner.
	if _, err := exec.LookPath("false"); err == nil {
		err := exec.Command("false").Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Skip("false didn't yield ExitError, skipping")
		}
		if got := exitCode(err); got != 1 {
			t.Fatalf("false exit code: got %d want 1", got)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
