// Package smoke is the Tier-2 regression suite per
// project_regression_testing_plan.md. One test that drives a real
// `duragraph serve` subprocess against testcontainers Postgres + NATS,
// exercises the happy path of the public REST API, and asserts that
// events flow through the outbox → JetStream pipeline end-to-end.
//
// Unlike tests/e2e/ (which is a separate Go module orchestrated by
// docker-compose) this package lives in the main module so it can
// share testcontainers deps with the rest of the unit tests and runs
// in standard `go test ./...` — no build tag, no bash orchestrator.
//
// Cost: ~25 s on a warm cache (binary build cached, container images
// cached). First run on a fresh machine pulls ~80 MB of images.
package smoke_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// engineBin is the path to the built duragraph binary. Populated once
// per test binary by TestMain — the build is cached so repeated runs
// are fast.
var (
	engineBin string
	buildOnce sync.Once
	buildErr  error

	pgOnce sync.Once
	pgErr  error
	pgHost string
	pgPort int
	pgUser = "duragraph_smoke"
	pgPass = "duragraph_smoke"
	pgDB   = "duragraph_smoke"

	natsOnce sync.Once
	natsErr  error
	natsURL  string
)

// buildEngine compiles cmd/duragraph to a temp binary so the smoke
// test can spawn it without paying the `go run` recompile penalty on
// each invocation. Cached via sync.Once.
func buildEngine(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		repoRoot, err := findRepoRoot()
		if err != nil {
			buildErr = err
			return
		}
		dir, err := os.MkdirTemp("", "duragraph-smoke-bin-*")
		if err != nil {
			buildErr = err
			return
		}
		bin := filepath.Join(dir, "duragraph")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/duragraph")
		cmd.Dir = repoRoot
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("go build duragraph: %v\n%s", err, out)
			return
		}
		engineBin = bin
	})
	if buildErr != nil {
		t.Fatalf("engine build: %v", buildErr)
	}
	return engineBin
}

// findRepoRoot walks up from this test file until it finds the
// top-level go.mod (the one declaring module github.com/duragraph/duragraph,
// distinct from tests/e2e/go.mod). Lets the smoke test run from
// anywhere `go test` cares to invoke it.
func findRepoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot resolve smoke_test.go path")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ {
		gomod := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(gomod); err == nil {
			if strings.Contains(string(data), "module github.com/duragraph/duragraph\n") {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not locate main module go.mod from smoke test")
}

// setupPostgres lazy-starts a postgres:15 container the smoke test
// will point the engine at via DB_* env vars. The engine applies tenant
// migrations itself on startup, so all we hand out is host/port/creds.
func setupPostgres(t *testing.T) (host string, port int) {
	t.Helper()
	pgOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		c, err := tcpostgres.Run(ctx,
			"postgres:15-alpine",
			tcpostgres.WithDatabase(pgDB),
			tcpostgres.WithUsername(pgUser),
			tcpostgres.WithPassword(pgPass),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
		)
		if err != nil {
			pgErr = fmt.Errorf("start postgres: %w", err)
			return
		}
		h, err := c.Host(ctx)
		if err != nil {
			pgErr = err
			return
		}
		pgHost = h
		mapped, err := c.MappedPort(ctx, "5432/tcp")
		if err != nil {
			pgErr = err
			return
		}
		p, err := strconv.Atoi(mapped.Port())
		if err != nil {
			pgErr = fmt.Errorf("parse mapped port %q: %w", mapped, err)
			return
		}
		pgPort = p
	})
	if pgErr != nil {
		t.Fatalf("postgres testcontainer: %v", pgErr)
	}
	return pgHost, pgPort
}

func setupNATS(t *testing.T) string {
	t.Helper()
	natsOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		c, err := tcnats.Run(ctx,
			"nats:2.10-alpine",
			testcontainers.WithCmdArgs("--jetstream"),
		)
		if err != nil {
			natsErr = err
			return
		}
		url, err := c.ConnectionString(ctx)
		if err != nil {
			natsErr = err
			return
		}
		natsURL = url
	})
	if natsErr != nil {
		t.Fatalf("nats testcontainer: %v", natsErr)
	}
	return natsURL
}

// freeAPIPort grabs a kernel-assigned free TCP port for the engine's
// HTTP listener so parallel test runs can coexist. TOCTOU window
// between close + re-bind is negligible for a self-contained test.
func freeAPIPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// TestSmoke_AssistantCreatedFiresEvent is the Tier-2 regression: a
// real `duragraph serve` boots against testcontainers Postgres + NATS,
// we POST an assistant via the REST API, and we expect the
// `assistant.created` event to land on the NATS bus via the outbox
// relay. Exercises the FULL stack in one test:
//
//   - engine boot + migrator + relay startup
//   - REST handler → event store → outbox row + pg_notify
//   - relay's LISTEN wake-up → JetStream publish
//   - subscriber receives matching event_id
func TestSmoke_AssistantCreatedFiresEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test skipped in -short mode")
	}

	bin := buildEngine(t)
	host, port := setupPostgres(t)
	natsAddr := setupNATS(t)
	apiPort := freeAPIPort(t)

	// Spawn the engine. AUTH_ENABLED=false keeps the smoke test from
	// having to authenticate; AUTH_PASSWORD_ENABLED=false keeps the
	// /api/auth/* routes off (we're not testing auth here).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "serve")
	cmd.Env = append(os.Environ(),
		"PORT="+strconv.Itoa(apiPort),
		"HOST=127.0.0.1",
		"DB_MODE=external",
		"DB_HOST="+host,
		"DB_PORT="+strconv.Itoa(port),
		"DB_USER="+pgUser,
		"DB_PASSWORD="+pgPass,
		"DB_NAME="+pgDB,
		"DB_SSLMODE=disable",
		"NATS_MODE=external",
		"NATS_URL="+natsAddr,
		"AUTH_ENABLED=false",
		"AUTH_PASSWORD_ENABLED=false",
		"MIGRATOR_PLATFORM_ENABLED=false",
	)
	cmd.Stdout = newPrefixWriter("[engine] ")
	cmd.Stderr = newPrefixWriter("[engine] ")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		_ = cmd.Wait()
	})

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", apiPort)
	waitForReady(t, apiURL)

	// Subscribe to NATS BEFORE issuing the create so we can't race
	// past the publish.
	nc, err := natsgo.Connect(natsAddr)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	events := make(chan map[string]any, 16)
	sub, err := nc.Subscribe("duragraph.>", func(m *natsgo.Msg) {
		var env map[string]any
		if err := json.Unmarshal(m.Data, &env); err != nil {
			return
		}
		select {
		case events <- env:
		default:
			// Drop if buffer full — we only need one match.
		}
	})
	if err != nil {
		t.Fatalf("nats subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Create an assistant via the public REST API.
	body, err := json.Marshal(map[string]any{
		"name":     "smoke-assistant",
		"graph_id": "hello_world",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp, err := http.Post(apiURL+"/api/v1/assistants", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("POST /api/v1/assistants: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("create assistant: HTTP %d, body=%s", resp.StatusCode, respBody)
	}

	var created struct {
		AssistantID string `json:"assistant_id"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, respBody)
	}
	if created.AssistantID == "" {
		t.Fatalf("no assistant_id in response: %s", respBody)
	}

	// Wait for the assistant.created event to flow through outbox →
	// relay → JetStream → core-NATS subscriber. With Phase 3's
	// LISTEN/NOTIFY this should be sub-100ms in practice; 5s is
	// generous headroom.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case env := <-events:
			eventType, _ := env["event_type"].(string)
			aggID, _ := env["aggregate_id"].(string)
			if eventType == "assistant.created" && aggID == created.AssistantID {
				return // success
			}
			// Different event — keep draining.
		case <-deadline:
			t.Fatalf("did not receive assistant.created event for %s within 5s", created.AssistantID)
		}
	}
}

// waitForReady polls /health every 200ms until 200 OK or timeout.
// The engine's startup includes migrator + relay listen + dashboard
// embed extract; takes ~3-5s on a warm container.
func waitForReady(t *testing.T, apiURL string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(apiURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("engine did not become ready at %s within 60s", apiURL)
}

// prefixWriter wraps stdout/stderr from the spawned engine so its log
// lines are clearly attributed in test output rather than mixed with
// test framework output.
type prefixWriter struct {
	prefix string
	buf    strings.Builder
}

func newPrefixWriter(prefix string) *prefixWriter {
	return &prefixWriter{prefix: prefix}
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	s := w.buf.String()
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			w.buf.Reset()
			w.buf.WriteString(s)
			break
		}
		fmt.Fprintln(os.Stderr, w.prefix+s[:i])
		s = s[i+1:]
	}
	return len(p), nil
}
