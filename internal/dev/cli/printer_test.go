package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintJSON_Indented(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSON(&buf, map[string]any{"k": "v", "n": 1}); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	got := buf.String()
	// MarshalIndent sorts map keys alphabetically — assert the literal
	// indented shape so a future "compact mode" doesn't silently break
	// the operator-facing format without a test failure.
	want := "{\n  \"k\": \"v\",\n  \"n\": 1\n}\n"
	if got != want {
		t.Fatalf("PrintJSON output mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestPrintJSON_Error(t *testing.T) {
	// Channels are not JSON-marshalable; PrintJSON should return the
	// underlying marshal error rather than panicking or writing garbage.
	if err := PrintJSON(&bytes.Buffer{}, make(chan int)); err == nil {
		t.Fatal("PrintJSON(chan): expected error, got nil")
	}
}

func TestPrintEvent_PrefixesType(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintEvent(&buf, "run.started", map[string]any{"run_id": "abc"}); err != nil {
		t.Fatalf("PrintEvent: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "// run.started\n") {
		t.Fatalf("PrintEvent: expected prefix line, got: %q", out)
	}
	if !strings.Contains(out, `"run_id": "abc"`) {
		t.Fatalf("PrintEvent: expected payload, got: %q", out)
	}
}

func TestPrintEvent_NoTypeOmitsPrefix(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintEvent(&buf, "", map[string]any{"x": 1}); err != nil {
		t.Fatalf("PrintEvent: %v", err)
	}
	if strings.HasPrefix(buf.String(), "//") {
		t.Fatalf("PrintEvent: expected no prefix line for empty type, got: %q", buf.String())
	}
}
