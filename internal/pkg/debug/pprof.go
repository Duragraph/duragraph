// Package debug exposes the stdlib net/http/pprof handlers on a
// separate localhost-bound listener for contributor profiling. It is
// deliberately separated from the main API router so:
//
//   - pprof handlers cannot leak through accidentally if someone wires
//     the default ServeMux into a public listener
//   - the listener can be bound to 127.0.0.1 (or empty / off entirely)
//     regardless of where the API server binds, so production deploys
//     don't accidentally expose heap dumps to the internet
//   - enabling/disabling is a single env var, no flag plumbing through
//     cobra or config.Load needed
//
// Workflow:
//
//	`duragraph dev` auto-enables this at 127.0.0.1:6060 (see dev.go's
//	applyDevEnvDefaults — sets DURAGRAPH_PPROF_ADDR if unset).
//
//	`duragraph serve` leaves it off. Operators opt in with:
//	    DURAGRAPH_PPROF_ADDR=127.0.0.1:6060 duragraph serve
//	never expose this on 0.0.0.0 — see security note below.
//
// Contributor usage once running:
//
//	go tool pprof http://localhost:6060/debug/pprof/heap
//	  (pprof) top10
//	  (pprof) list <function>
//	  (pprof) web        # flamegraph in browser
//
//	curl http://localhost:6060/debug/pprof/goroutine?debug=2 | grep -c '^goroutine'
//	  prints the goroutine count — diff across snapshots to find leaks
//
// Security note: the pprof endpoints expose heap state (sensitive
// data may be retained in memory), accept long-running profile
// requests (DoS via worker exhaustion), and reveal binary symbols
// (useful for vulnerability scanning). NEVER bind to 0.0.0.0 or a
// public interface — the unconditional 127.0.0.1 default in dev
// mode is intentional.
package debug

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"
)

// StartPprof spins up a separate net/http listener on addr serving
// the stdlib net/http/pprof handlers. Returns nil if addr is empty
// (pprof disabled). Returns an error only on misconfiguration —
// the listener runs in a goroutine, so listener errors after Start
// returns are slog.Error()'d but not returned.
//
// Defensive: rejects non-localhost binds with a slog.Warn + a guard
// rejection. Operators who genuinely need a remote-reachable pprof
// can set DURAGRAPH_PPROF_ALLOW_PUBLIC=true to bypass — but the
// default-deny is intentional.
func StartPprof(ctx context.Context, addr string) error {
	if addr == "" {
		return nil
	}

	// Guard against accidental public binds.
	if !isLocalhostBind(addr) && !envAllowsPublic() {
		slog.Warn("pprof listener refused public bind",
			"addr", addr,
			"hint", "set DURAGRAPH_PPROF_ALLOW_PUBLIC=true to override, but heap dumps + DoS risk")
		return errors.New("pprof: refused public bind (set DURAGRAPH_PPROF_ALLOW_PUBLIC=true to override)")
	}

	// Register pprof handlers on a fresh mux so they NEVER end up on
	// the default ServeMux. If we used `import _ "net/http/pprof"`
	// instead, the handlers would be registered on the default mux
	// at init time — meaning any future code that wires
	// http.DefaultServeMux into a public listener (third-party
	// integration, devtools, anything) would expose pprof too. A
	// scoped mux is belt-and-suspenders.
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second, // belt-and-suspenders against slowloris
	}

	go func() {
		slog.Info("pprof listener started", "addr", addr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("pprof listener failed", "err", err)
		}
	}()

	// Graceful shutdown when the parent context is cancelled. Mirrors
	// the engine's main HTTP server lifecycle so pprof doesn't
	// outlive a graceful Ctrl-C.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	return nil
}

// isLocalhostBind returns true if addr resolves to a loopback bind.
// We accept ":<port>" (binds to all interfaces — technically not
// loopback) only when the user has explicitly set the public-allow
// env var, otherwise it's caught by the public-bind guard.
func isLocalhostBind(addr string) bool {
	// Strip optional ":port" suffix; pull out the host.
	host := addr
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		host = addr[:idx]
	}
	switch host {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	default:
		return false
	}
}

func envAllowsPublic() bool {
	// Read at call time (not init) so tests can flip it via t.Setenv.
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DURAGRAPH_PPROF_ALLOW_PUBLIC"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
