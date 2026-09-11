package server_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dgserver "github.com/duragraph/duragraph/controlplane/server"
)

// stubDashboard stands in for the built React app. Using a synthetic
// filesystem rather than the real embed keeps these assertions about
// routing, which is what the bug was — the real dashboard/dist carries a
// placeholder index.html in git, so its contents prove nothing either way.
func stubDashboard() fstest.MapFS {
	return fstest.MapFS{
		"index.html":      &fstest.MapFile{Data: []byte("<html>DASHBOARD</html>")},
		"assets/app.js":   &fstest.MapFile{Data: []byte("console.log(1)")},
		"favicon.svg":     &fstest.MapFile{Data: []byte("<svg/>")},
		"nested/page.txt": &fstest.MapFile{Data: []byte("nested")},
	}
}

// startDashboardServer boots a control plane with the stub UI mounted and
// returns its base URL.
func startDashboardServer(t *testing.T, ctx context.Context) string {
	t.Helper()

	addr, err := freeAddr()
	if err != nil {
		t.Fatalf("free addr: %v", err)
	}
	srv, err := dgserver.New(ctx, dgserver.Config{
		Addr:         addr,
		TenantDSN:    tenantDSN,
		Migrate:      false, // applied once in TestMain
		DashboardFS:  stubDashboard(),
		DrainTimeout: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	go func() { _ = srv.Run(ctx) }()
	waitForListen(t, addr)
	return "http://" + addr
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestDashboardIsServed is the regression test for the gap this closes:
// the rebuilt control plane mounted no static UI at all, so it answered
// the API and nothing at "/". A route-by-route diff against the legacy
// server could not surface it, because the dashboard is not a route.
func TestDashboardIsServed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := startDashboardServer(t, ctx)

	t.Run("root serves index.html", func(t *testing.T) {
		code, body := get(t, base+"/")
		if code != http.StatusOK {
			t.Fatalf("GET /: want 200, got %d", code)
		}
		if !strings.Contains(body, "DASHBOARD") {
			t.Errorf("GET / did not serve the dashboard: %q", body)
		}
	})

	t.Run("static assets are served", func(t *testing.T) {
		code, body := get(t, base+"/assets/app.js")
		if code != http.StatusOK {
			t.Fatalf("want 200, got %d", code)
		}
		if !strings.Contains(body, "console.log") {
			t.Errorf("asset body wrong: %q", body)
		}
	})

	t.Run("unknown path falls back to index.html for client routing", func(t *testing.T) {
		// TanStack Router owns these paths in the browser; the server has
		// to hand back the shell rather than 404, or a deep link breaks.
		for _, p := range []string{"/playground", "/builder", "/deployments", "/inspector"} {
			code, body := get(t, base+p)
			if code != http.StatusOK {
				t.Errorf("GET %s: want 200, got %d", p, code)
				continue
			}
			if !strings.Contains(body, "DASHBOARD") {
				t.Errorf("GET %s did not fall back to the SPA shell", p)
			}
		}
	})
}

// TestDashboardDoesNotShadowTheAPI pins the boundary between the SPA
// catch-all and the API.
//
// Note what this does NOT test: registration order. Echo prefers static
// and parameterised routes over a catch-all whichever order they are
// registered in — mounting the dashboard before the API still leaves
// every API route reachable, which I confirmed rather than assumed.
//
// What it does catch is the handler's /api/ guard. Without it an
// unmatched /api/... path returns 200 and the SPA shell to a client
// expecting JSON, which is considerably more confusing than a 404.
func TestDashboardDoesNotShadowTheAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := startDashboardServer(t, ctx)

	t.Run("system routes still answer", func(t *testing.T) {
		for _, p := range []string{"/ok", "/info"} {
			code, body := get(t, base+p)
			if code != http.StatusOK {
				t.Errorf("GET %s: want 200, got %d", p, code)
			}
			if strings.Contains(body, "DASHBOARD") {
				t.Errorf("GET %s was swallowed by the SPA catch-all", p)
			}
		}
	})

	t.Run("api routes still answer", func(t *testing.T) {
		code, body := get(t, base+"/api/v1/assistants/count")
		if strings.Contains(body, "DASHBOARD") {
			t.Fatalf("GET /api/v1/assistants/count served the SPA shell (status %d)", code)
		}
		// The status depends on request validation; what matters is that a
		// real handler answered rather than the catch-all.
		if code == http.StatusNotFound {
			t.Errorf("assistants/count returned 404 — route not reached")
		}
	})

	t.Run("unmatched api paths get a clean 404, not the SPA", func(t *testing.T) {
		code, body := get(t, base+"/api/v1/definitely-not-a-route")
		if code != http.StatusNotFound {
			t.Errorf("want 404, got %d", code)
		}
		if strings.Contains(body, "DASHBOARD") {
			t.Error("unmatched /api/ path fell through to the SPA shell")
		}
	})
}

// TestDashboardAbsentDoesNotBlockTheAPI: a control plane with no usable UI
// must still serve. Failing to boot over a missing dashboard would turn a
// cosmetic problem into an outage for a headless deployment.
func TestDashboardAbsentDoesNotBlockTheAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := freeAddr()
	if err != nil {
		t.Fatalf("free addr: %v", err)
	}
	srv, err := dgserver.New(ctx, dgserver.Config{
		Addr:      addr,
		TenantDSN: tenantDSN,
		Migrate:   false,
		// No index.html — mounting must fail, and be survivable.
		DashboardFS:  fstest.MapFS{"readme.txt": &fstest.MapFile{Data: []byte("x")}},
		DrainTimeout: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("server must start without a usable dashboard: %v", err)
	}
	defer srv.Close()

	go func() { _ = srv.Run(ctx) }()
	waitForListen(t, addr)

	base := "http://" + addr
	code, _ := get(t, base+"/ok")
	if code != http.StatusOK {
		t.Errorf("API must serve without a dashboard: /ok returned %d", code)
	}
}
