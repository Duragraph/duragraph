package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	dgserver "github.com/duragraph/duragraph/controlplane/server"
)

// TestHealthLegacyCompatibility checks the actual v2 HTTP router (including
// the dashboard catch-all), not just the endpoint handler. A broken database
// must not take the legacy liveness probe down with the readiness probe.
func TestHealthLegacyCompatibility(t *testing.T) {
	addr, err := freeAddr()
	if err != nil {
		t.Fatalf("free addr: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := dgserver.New(ctx, dgserver.Config{
		Addr:         addr,
		TenantDSN:    "postgres://t:t@127.0.0.1:1/tenant?sslmode=disable&connect_timeout=1",
		Migrate:      false,
		DashboardFS:  stubDashboard(),
		DrainTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer srv.Close()
	go func() { _ = srv.Run(ctx) }()
	waitForListen(t, addr)
	base := "http://" + addr

	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health: want 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("GET /health content type: %q", got)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GET /health JSON: %v", err)
	}
	want := map[string]string{"status": "healthy", "version": "2.0.0-ddd"}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("GET /health: got %#v, want %#v", body, want)
	}

	code, _ := get(t, base+"/ok")
	if code != http.StatusServiceUnavailable {
		t.Errorf("GET /ok with unavailable DB: want 503, got %d", code)
	}
	code, _ = get(t, base+"/api/v1/health")
	if code != http.StatusNotFound {
		t.Errorf("GET /api/v1/health: want 404, got %d", code)
	}
}
