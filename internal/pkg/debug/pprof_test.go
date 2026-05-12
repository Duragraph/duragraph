package debug

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStartPprof_EmptyAddrIsNoOp(t *testing.T) {
	// Empty addr → pprof disabled. Should return nil without starting
	// any listener.
	if err := StartPprof(context.Background(), ""); err != nil {
		t.Fatalf("empty addr: want nil err, got %v", err)
	}
}

func TestStartPprof_LocalhostBindServesHeapHandler(t *testing.T) {
	// Bind to :0 → kernel picks a free port. We need a deterministic
	// localhost-prefixed addr so isLocalhostBind accepts it; ":0"
	// would fail the public-bind guard, so we explicitly bind to
	// 127.0.0.1:0 instead. The Listen behaviour is the same.
	addr := "127.0.0.1:46060"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := StartPprof(ctx, addr); err != nil {
		t.Fatalf("StartPprof: %v", err)
	}

	// Give the goroutine a moment to actually bind. ListenAndServe
	// runs in a goroutine; without this the first request can race
	// the listener and connection-refuse.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := http.Get("http://" + addr + "/debug/pprof/")
		if err == nil {
			conn.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// `?debug=1` gives the human-readable text profile; without it,
	// the response is a pprof-format binary blob (also 200, but
	// trickier to verify in a test).
	resp, err := http.Get("http://" + addr + "/debug/pprof/heap?debug=1")
	if err != nil {
		t.Fatalf("GET heap: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heap status: want 200, got %d", resp.StatusCode)
	}
}

func TestStartPprof_RefusesPublicBindByDefault(t *testing.T) {
	// 0.0.0.0 binds are rejected unless DURAGRAPH_PPROF_ALLOW_PUBLIC
	// is set. Default-deny is the whole reason this package exists.
	err := StartPprof(context.Background(), "0.0.0.0:46061")
	if err == nil {
		t.Fatalf("public bind: want error, got nil")
	}
	if !strings.Contains(err.Error(), "refused public bind") {
		t.Errorf("error message should mention 'refused public bind', got: %v", err)
	}
}

func TestStartPprof_AllowsPublicBindWithOverride(t *testing.T) {
	t.Setenv("DURAGRAPH_PPROF_ALLOW_PUBLIC", "true")
	// Use a port we won't actually serve on for the test — just
	// verifying the guard doesn't reject the call. The listener
	// itself will fail or succeed depending on port availability,
	// but StartPprof returns nil regardless because the listener
	// runs in a goroutine.
	if err := StartPprof(context.Background(), "0.0.0.0:0"); err != nil {
		t.Fatalf("override should permit: %v", err)
	}
}

func TestIsLocalhostBind(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:6060": true,
		"localhost:6060": true,
		"[::1]:6060":     true,
		"::1:6060":       true,
		"0.0.0.0:6060":   false,
		":6060":          false,
		"10.0.0.1:6060":  false,
		"example.com:80": false,
	}
	for addr, want := range cases {
		got := isLocalhostBind(addr)
		if got != want {
			t.Errorf("isLocalhostBind(%q) = %v, want %v", addr, got, want)
		}
	}
}
