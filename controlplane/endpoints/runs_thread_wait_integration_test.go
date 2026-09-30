package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duragraph/duragraph/controlplane/nats"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func threadWaitFixture(t *testing.T) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, events, outbox, event_streams, assistants, threads CASCADE"); err != nil {
		t.Fatal(err)
	}
	var assistantID, threadID uuid.UUID
	if err := testPool.QueryRow(ctx, `INSERT INTO assistants (graph_id, name) VALUES ('hello_world', 'wait-test') RETURNING id`).Scan(&assistantID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	newStreamTestServer().RegisterRuns(e.Group("/api/v1"))
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return threadID, assistantID, srv.URL + "/api/v1/threads/" + threadID.String() + "/runs/wait"
}

func postThreadWait(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func waitForThreadRun(t *testing.T, threadID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var rid uuid.UUID
		if err := testPool.QueryRow(ctx, `SELECT id FROM runs WHERE thread_id=$1`, threadID).Scan(&rid); err == nil {
			return rid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("thread wait did not persist a run")
	return uuid.Nil
}

func TestThreadWaitTerminal(t *testing.T) {
	for _, tc := range []struct {
		dbStatus, event, apiStatus string
		output, failure            string
	}{
		{"completed", "run.completed", "success", `{"answer":["one",2]}`, ""},
		{"failed", "run.failed", "error", "", "node failed: unavailable"},
		{"cancelled", "run.cancelled", "error", "", ""},
	} {
		t.Run(tc.dbStatus, func(t *testing.T) {
			threadID, assistantID, url := threadWaitFixture(t)
			body := `{"assistant_id":"` + assistantID.String() + `","input":{"message":"hi"},"metadata":{"source":"test"},"interrupt_before":["node"]}`
			result := make(chan *http.Response, 1)
			go func() {
				resp, err := http.Post(url, "application/json", strings.NewReader(body))
				if err != nil {
					t.Errorf("POST wait: %v", err)
					result <- nil
					return
				}
				result <- resp
			}()
			rid := waitForThreadRun(t, threadID)
			select {
			case resp := <-result:
				if resp != nil {
					resp.Body.Close()
				}
				t.Fatal("wait returned before terminal event")
			default:
			}
			var storedThread, storedAssistant uuid.UUID
			var input, metadata, kwargs []byte
			if err := testPool.QueryRow(context.Background(), `SELECT thread_id, assistant_id, input, metadata, kwargs FROM runs WHERE id=$1`, rid).
				Scan(&storedThread, &storedAssistant, &input, &metadata, &kwargs); err != nil {
				t.Fatal(err)
			}
			if storedThread != threadID || storedAssistant != assistantID || !strings.Contains(string(input), "hi") || !strings.Contains(string(metadata), "test") || !strings.Contains(string(kwargs), "interrupt_before") {
				t.Fatalf("stateful run lost request fields: thread=%s assistant=%s input=%s metadata=%s kwargs=%s", storedThread, storedAssistant, input, metadata, kwargs)
			}
			var events, outbox int
			if err := testPool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='run.created'), (SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_type='run.created')`, rid).Scan(&events, &outbox); err != nil || events != 1 || outbox != 1 {
				t.Fatalf("run.created transaction: events=%d outbox=%d err=%v", events, outbox, err)
			}
			var payload []byte
			if err := testPool.QueryRow(context.Background(), `SELECT payload FROM events WHERE aggregate_id=$1 AND event_type='run.created'`, rid).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			var event struct {
				ThreadID uuid.UUID `json:"thread_id"`
			}
			if err := json.Unmarshal(payload, &event); err != nil || event.ThreadID != threadID {
				t.Fatalf("run.created must identify thread: payload=%s err=%v", payload, err)
			}
			var output any
			if tc.output != "" {
				output = []byte(tc.output)
			}
			var failure any
			if tc.failure != "" {
				failure = tc.failure
			}
			if _, err := testPool.Exec(context.Background(), `UPDATE runs SET status=$2, output=$3, error=$4 WHERE id=$1`, rid, tc.dbStatus, output, failure); err != nil {
				t.Fatal(err)
			}
			pub := nats.NewPublisher(mustJS(t))
			if err := pub.PublishWithID(context.Background(), nats.SubjectFor(tc.event), uuid.NewString(), envelopeFor(rid, tc.event, `{}`)); err != nil {
				t.Fatal(err)
			}
			select {
			case resp := <-result:
				if resp == nil {
					t.Fatal("POST failed")
				}
				defer resp.Body.Close()
				var got map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != http.StatusOK || got["status"] != tc.apiStatus || got["thread_id"] != threadID.String() || got["run_id"] != rid.String() {
					t.Fatalf("unexpected wait result: %d %+v", resp.StatusCode, got)
				}
				if tc.output != "" {
					value, ok := got["output"].(map[string]any)
					if !ok {
						t.Fatalf("final output missing or not JSON: %+v", got["output"])
					}
					answer, ok := value["answer"].([]any)
					if !ok || len(answer) != 2 || answer[0] != "one" || answer[1] != float64(2) {
						t.Fatalf("final output corrupted: %+v", got["output"])
					}
				} else if value, ok := got["output"]; !ok || value != nil {
					t.Fatalf("missing output must be null: %+v", got)
				}
				if tc.failure != "" {
					if got["error"] != tc.failure {
						t.Fatalf("failure message missing: %+v", got)
					}
				} else if value, ok := got["error"]; !ok || value != nil {
					t.Fatalf("missing failure must be null: %+v", got)
				}
				if want := "/api/v1/threads/" + threadID.String() + "/runs/" + rid.String(); resp.Header.Get("Content-Location") != want {
					t.Errorf("Content-Location = %q, want %q", resp.Header.Get("Content-Location"), want)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("wait did not return on terminal event")
			}
		})
	}
}

func TestThreadWaitTimeout(t *testing.T) {
	threadID, assistantID, url := threadWaitFixture(t)
	resp := postThreadWait(t, url+"?timeout=100ms", `{"assistant_id":"`+assistantID.String()+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("want 504, got %d", resp.StatusCode)
	}
	rid := waitForThreadRun(t, threadID)
	var status string
	if err := testPool.QueryRow(context.Background(), `SELECT status FROM runs WHERE id=$1`, rid).Scan(&status); err != nil || status != "queued" {
		t.Fatalf("timed-out wait must not cancel run: status=%s err=%v", status, err)
	}
}

func TestThreadWaitRejectsInvalidRequests(t *testing.T) {
	threadID, assistantID, url := threadWaitFixture(t)
	for _, tc := range []struct {
		name, url, body string
		status          int
	}{
		{"bad id", strings.Replace(url, threadID.String(), "bad-id", 1), `{"assistant_id":"` + assistantID.String() + `"}`, 400},
		{"missing thread", strings.Replace(url, threadID.String(), uuid.NewString(), 1), `{"assistant_id":"` + assistantID.String() + `"}`, 404},
		{"invalid timeout", url + "?timeout=oops", `{"assistant_id":"` + assistantID.String() + `"}`, 400},
		{"invalid interrupt", url, `{"assistant_id":"` + assistantID.String() + `","interrupt_before":42}`, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := postThreadWait(t, tc.url, tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("want %d got %d", tc.status, resp.StatusCode)
			}
		})
	}
	var count int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM runs WHERE thread_id=$1`, threadID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid requests created runs: %d (%v)", count, err)
	}
}

func TestThreadWaitRequiresNATS(t *testing.T) {
	e := echo.New()
	(&Server{Tenant: testPool}).RegisterRuns(e.Group("/api/v1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+uuid.NewString()+"/runs/wait", strings.NewReader(`{}`))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", rec.Code, rec.Body.String())
	}
}
