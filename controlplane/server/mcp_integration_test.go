package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	dgserver "github.com/duragraph/duragraph/controlplane/server"
)

func TestMCPMountedAtRoot(t *testing.T) {
	addr, err := freeAddr()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := dgserver.New(context.Background(), dgserver.Config{TenantDSN: tenantDSN, Migrate: false, Addr: addr})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Run(ctx) }()
	waitForListen(t, addr)
	for _, path := range []string{"/mcp", "/mcp/"} {
		res, err := http.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s: %d", path, res.StatusCode)
		}
		req, err := http.NewRequest(http.MethodPost, "http://"+addr+path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Content-Type", "application/json")
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		// No platform database / JWT: a reachable MCP route must fail closed.
		if res.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("POST %s: %d", path, res.StatusCode)
		}
	}
}
