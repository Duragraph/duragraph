package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultEngineURL is the engine URL the CLI assumes when neither the
// --engine flag nor the DURAGRAPH_URL environment variable is set. Mirrors
// the port `duragraph dev` defaults to (binary-modes.yml § subcommands.dev).
const DefaultEngineURL = "http://localhost:8081"

// Client is a thin HTTP wrapper around the engine's REST + SSE API. It
// is intentionally minimal — no retries, no auth header injection
// (Phase 7 targets the no-auth `duragraph dev` flow), no connection
// pooling beyond Go's default. Future PRs can layer tenant-JWT auth or
// retry policy on top without changing call sites.
type Client struct {
	http   *http.Client
	engine string
}

// NewClient returns a Client targeting engineURL. The URL is normalised
// (trailing slash trimmed) so callers can append paths starting with
// "/api/v1/..." without worrying about double slashes.
func NewClient(engineURL string) *Client {
	return &Client{
		// No timeout on the http.Client itself: the SSE call needs a
		// long-lived connection and per-call timeouts are applied via
		// context.Context instead.
		http:   &http.Client{},
		engine: strings.TrimRight(engineURL, "/"),
	}
}

// EngineURL returns the normalised base URL the client targets. Useful
// for log messages that orient the operator on which engine they hit.
func (c *Client) EngineURL() string { return c.engine }

// GetRun fetches `GET /api/v1/runs/<runID>` and returns the raw JSON
// body. The body is returned undecoded so the caller can pretty-print it
// straight to stdout — `runs get` does not need a typed view.
//
// A 404 is surfaced as a sentinel error so callers can render a clean
// "run not found" message rather than dumping the raw error envelope.
func (c *Client) GetRun(ctx context.Context, runID string) ([]byte, error) {
	url := c.engine + "/api/v1/runs/" + runID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build GET %s: %w", url, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("read GET %s body: %w", url, readErr)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrRunNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// ErrRunNotFound is returned by GetRun when the engine responds 404.
// Sentinel rather than a typed error because the caller only needs to
// distinguish "missing" from "other failure".
var ErrRunNotFound = errors.New("run not found")

// CreateRunResponse mirrors dto.CreateRunResponse for the fields the CLI
// surfaces. Defined locally rather than importing the dto package to
// keep this CLI helper from picking up the engine's transitive deps
// (echo, persistence, etc.) and inflating the binary.
type CreateRunResponse struct {
	RunID       string `json:"run_id"`
	ThreadID    string `json:"thread_id"`
	AssistantID string `json:"assistant_id"`
	Status      string `json:"status"`
}

// CreateRun POSTs `/api/v1/runs` with the given body and returns the
// decoded response. The engine accepts a raw JSON object here — the
// caller is responsible for shape-validating `body` before sending so
// invalid inputs fail fast on the client side rather than producing a
// 400 round-trip.
func (c *Client) CreateRun(ctx context.Context, body any) (*CreateRunResponse, error) {
	url := c.engine + "/api/v1/runs"
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal create-run body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build POST %s: %w", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("read POST %s body: %w", url, readErr)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("POST %s: %s: %s", url, resp.Status, strings.TrimSpace(string(respBody)))
	}

	var out CreateRunResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode create-run response: %w", err)
	}
	return &out, nil
}

// terminalStatuses are the run states at which `runs trigger --wait`
// stops polling. Mirrors the run-aggregate state machine in
// internal/domain/run/.
var terminalStatuses = map[string]struct{}{
	"completed": {},
	"failed":    {},
	"cancelled": {},
}

// WaitForRun polls GetRun every interval until the run reaches a
// terminal state (completed | failed | cancelled) or ctx is cancelled.
// Returns the final raw JSON body so the caller can print the same
// payload `runs get` would.
//
// Polling is deliberately simple — no exponential backoff, no event
// subscription. `runs trigger --wait` is an interactive convenience for
// `duragraph dev`, not a production polling loop. If a future workflow
// needs sub-second latency, switching to NATS-subscribe is a one-line
// change at the call site.
func (c *Client) WaitForRun(ctx context.Context, runID string, interval time.Duration) ([]byte, error) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		body, err := c.GetRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		var probe struct {
			Status string `json:"status"`
		}
		// A failed decode here is non-fatal: keep polling. The body is
		// still returned at terminal time, so the operator sees the
		// real shape regardless.
		_ = json.Unmarshal(body, &probe)
		if _, terminal := terminalStatuses[probe.Status]; terminal {
			return body, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// SSEEvent is one parsed Server-Sent Events frame. The engine's SSE
// stream uses only the `event:` and `data:` fields (formatter.go:55),
// so id/retry are intentionally ignored.
type SSEEvent struct {
	Type string
	Data []byte
}

// StreamRuns opens an SSE connection to the engine's run-event stream
// for threadID and invokes fn for each parsed event. Blocks until ctx
// is cancelled, the connection drops, or fn returns an error.
//
// Hand-rolled rather than pulling in github.com/r3labs/sse: the wire
// format here is trivial (line-prefixed event:/data:, blank-line frame
// separator) and we don't need reconnect-with-backoff or last-event-id
// resume — both of those are valuable for a real client SDK, but the
// CLI is operator-facing and the operator can re-run the command if
// the engine restarts.
//
// Endpoint: /api/v1/threads/<threadID>/stream — see stream.go's
// JoinThreadStream. There is no engine-side "all threads" SSE endpoint;
// for that flow the CLI uses NATS instead (see SubscribeRunEvents).
func (c *Client) StreamRuns(ctx context.Context, threadID string, fn func(SSEEvent) error) error {
	if threadID == "" {
		return errors.New("StreamRuns: threadID is required (use NATS path for all-threads tail)")
	}
	url := c.engine + "/api/v1/threads/" + threadID + "/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build SSE GET %s: %w", url, err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("SSE GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SSE GET %s: %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}
	return parseSSE(resp.Body, fn)
}

// parseSSE reads the SSE byte stream from r and dispatches one SSEEvent
// to fn per `event:`/`data:` block (delimited by a blank line). Pulled
// out as a free function so the test can drive it with an in-memory
// buffer without spinning up an httptest server.
//
// The engine never splits a single logical event across multiple data:
// lines — the formatter writes one data: line per frame (formatter.go:55) —
// but parseSSE handles multi-line data: anyway (joined with '\n', per
// the W3C SSE spec) so the helper is reusable for other SSE producers.
func parseSSE(r io.Reader, fn func(SSEEvent) error) error {
	scanner := bufio.NewScanner(r)
	// Default 64KiB buffer is too small for large workflow state
	// payloads. Bump to 1MiB which matches the engine's HTTP body size
	// cap on the producer side.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		eventType string
		dataLines [][]byte
	)
	dispatch := func() error {
		if eventType == "" && len(dataLines) == 0 {
			return nil
		}
		ev := SSEEvent{
			Type: eventType,
			Data: bytes.Join(dataLines, []byte("\n")),
		}
		eventType = ""
		dataLines = dataLines[:0]
		return fn(ev)
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		// Comment lines (": keepalive") — ignore.
		if line[0] == ':' {
			continue
		}
		// `field: value` (with optional space after colon).
		colon := bytes.IndexByte(line, ':')
		var field, value string
		if colon < 0 {
			field = string(line)
			value = ""
		} else {
			field = string(line[:colon])
			v := line[colon+1:]
			if len(v) > 0 && v[0] == ' ' {
				v = v[1:]
			}
			value = string(v)
		}
		switch field {
		case "event":
			eventType = value
		case "data":
			dataLines = append(dataLines, []byte(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read SSE: %w", err)
	}
	// Flush a trailing event that wasn't followed by a blank line.
	return dispatch()
}
