// Command worker-smoke exercises the published Go worker against a real
// duragraph serve --control-plane=v2 process. It creates an assistant, thread,
// and run via the public API, then waits for the SDK's HTTP claim worker to
// produce terminal output and a per-node checkpoint. The optional injected
// failure confirms the documented post-claim recovery gap without touching
// any existing run. Use only with an isolated throwaway control-plane database.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/duragraph/duragraph/go-sdk/client"
	"github.com/duragraph/duragraph/go-sdk/graph"
	"github.com/duragraph/duragraph/go-sdk/worker"
)

type node struct {
	key   string
	value any
}

func (n node) Execute(_ context.Context, state map[string]any) (map[string]any, error) {
	if state == nil {
		state = map[string]any{}
	}
	state[n.key] = n.value
	return state, nil
}

// failCheckpoint injects one HTTP 503 locally. It does not alter the server or
// depend on a mock endpoint: the rest of the run uses the real wire protocol.
type failCheckpoint struct {
	base  http.RoundTripper
	armed atomic.Bool
	hit   atomic.Bool
}

func (f *failCheckpoint) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.armed.Load() && req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/checkpoints") && f.hit.CompareAndSwap(false, true) {
		return &http.Response{StatusCode: 503, Status: "503 injected failure", Body: io.NopCloser(strings.NewReader(`{"error":"injected post-lease failure"}`)), Header: make(http.Header), Request: req}, nil
	}
	return f.base.RoundTrip(req)
}

func main() {
	base := flag.String("url", "http://127.0.0.1:18981", "isolated v2 control-plane URL")
	transient := flag.Bool("transient", false, "inject one checkpoint failure after a second run is claimed")
	flag.Parse()
	if err := run(*base, *transient); err != nil {
		fmt.Fprintln(os.Stderr, "smoke failed:", err)
		os.Exit(1)
	}
}

func run(base string, transient bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	graphID := fmt.Sprintf("go_sdk_smoke_%d", time.Now().UnixNano())
	g := graph.New[map[string]any](graphID)
	g.AddNode("first", node{"first", true}).AddNode("last", node{"answer", "smoke-ok"}).AddEdge("first", "last").SetEntrypoint("first")
	transport := &failCheckpoint{base: http.DefaultTransport}
	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	w := worker.New(g, worker.WithControlPlane(base), worker.WithClaimInterval(100*time.Millisecond), worker.WithHTTPClient(httpClient))
	workerCtx, stop := context.WithCancel(ctx)
	defer stop()
	workerDone := make(chan error, 1)
	go func() { workerDone <- w.Start(workerCtx) }()
	api := client.New(base, client.WithHTTPClient(httpClient))
	// Creating the run can race registration; claim starts only after it.
	a, err := api.CreateAssistant(ctx, client.CreateAssistantRequest{GraphID: graphID, Name: "Go SDK wire smoke"})
	if err != nil {
		return fmt.Errorf("create assistant: %w", err)
	}
	t, err := api.CreateThread(ctx)
	if err != nil {
		return fmt.Errorf("create thread: %w", err)
	}
	first, err := api.CreateRun(ctx, t.ID, client.CreateRunRequest{AssistantID: a.ID, Input: map[string]any{"input": "hello"}})
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	completed, err := api.WaitForRun(ctx, t.ID, first.ID, 100*time.Millisecond)
	if err != nil {
		return err
	}
	if completed.Status != "success" {
		return fmt.Errorf("run %s finished with status %q, expected success", first.ID, completed.Status)
	}
	// The public Run projection omits runs.output; the smoke script checks
	// the terminal output in the isolated Postgres row.
	checkpointURL := fmt.Sprintf("%s/api/v1/threads/%s/checkpoints/latest?run_id=%s", base, t.ID, first.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkpointURL, nil)
	if err != nil {
		return err
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		if err := response.Body.Close(); err != nil {
			return fmt.Errorf("close checkpoint response: %w", err)
		}
		return fmt.Errorf("checkpoint: HTTP %d", response.StatusCode)
	}
	var cp struct {
		CheckpointID int64 `json:"checkpoint_id"`
		Version      int   `json:"version"`
		State        struct {
			State map[string]any `json:"state"`
			Next  string         `json:"next"`
		} `json:"state"`
	}
	if err := json.NewDecoder(response.Body).Decode(&cp); err != nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			return fmt.Errorf("decode checkpoint: %w (also close: %v)", err, closeErr)
		}
		return err
	}
	if err := response.Body.Close(); err != nil {
		return fmt.Errorf("close checkpoint response: %w", err)
	}
	if cp.CheckpointID <= 0 || cp.Version != 2 || cp.State.Next != "" || cp.State.State["answer"] != "smoke-ok" {
		return fmt.Errorf("unexpected checkpoint: %+v", cp)
	}
	fmt.Printf("PASS terminal run=%s status=%s checkpoint_id=%d version=%d state=%v\n", first.ID, completed.Status, cp.CheckpointID, cp.Version, cp.State.State)
	if transient {
		transport.armed.Store(true)
		failed, err := api.CreateRun(ctx, t.ID, client.CreateRunRequest{AssistantID: a.ID, Input: map[string]any{"input": "failure-probe"}})
		if err != nil {
			return err
		}
		for !transport.hit.Load() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
		// Give the claim loop several opportunities to retry. The run stays
		// in_progress: it is not in the queued set claim selects from.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		stuck, err := api.GetRun(ctx, t.ID, failed.ID)
		if err != nil {
			return err
		}
		if stuck.Status != "running" {
			return fmt.Errorf("transient probe expected running, got %s", stuck.Status)
		}
		fmt.Printf("CONFIRMED post-lease transient failure: run=%s remains %s (no claim recovery)\n", failed.ID, stuck.Status)
	}
	stop()
	select {
	case e := <-workerDone:
		if e != nil && !errors.Is(e, context.Canceled) {
			return e
		}
	case <-time.After(5 * time.Second):
		return errors.New("worker did not stop")
	}
	return nil
}
