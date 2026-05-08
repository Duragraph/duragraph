package watch

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrefixWriterSingleLine(t *testing.T) {
	var buf bytes.Buffer
	pw := newPrefixWriter("[x] ", &buf)
	if _, err := pw.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "[x] hello\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPrefixWriterMultipleLinesOneWrite(t *testing.T) {
	var buf bytes.Buffer
	pw := newPrefixWriter("[a] ", &buf)
	if _, err := pw.Write([]byte("one\ntwo\nthree\n")); err != nil {
		t.Fatal(err)
	}
	want := "[a] one\n[a] two\n[a] three\n"
	if got := buf.String(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPrefixWriterPartialLineAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	pw := newPrefixWriter("[p] ", &buf)
	// First write ends without a newline — should buffer.
	if _, err := pw.Write([]byte("hel")); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no flush yet, got %q", buf.String())
	}
	// Second write completes the line.
	if _, err := pw.Write([]byte("lo\n")); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "[p] hello\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPrefixWriterMixedTail(t *testing.T) {
	var buf bytes.Buffer
	pw := newPrefixWriter("[m] ", &buf)
	// Two complete lines then a partial.
	if _, err := pw.Write([]byte("a\nb\nc")); err != nil {
		t.Fatal(err)
	}
	want := "[m] a\n[m] b\n"
	if got := buf.String(); got != want {
		t.Fatalf("first stage got %q want %q", got, want)
	}
	// Tail flushed via Flush — adds a synthetic newline.
	if err := pw.Flush(); err != nil {
		t.Fatal(err)
	}
	want = "[m] a\n[m] b\n[m] c\n"
	if got := buf.String(); got != want {
		t.Fatalf("after flush got %q want %q", got, want)
	}
}

func TestPrefixWriterFlushNoop(t *testing.T) {
	var buf bytes.Buffer
	pw := newPrefixWriter("[n] ", &buf)
	if err := pw.Flush(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("flush of empty buffer wrote %q", buf.String())
	}
	// After a complete line, Flush should still be a no-op.
	if _, err := pw.Write([]byte("done\n")); err != nil {
		t.Fatal(err)
	}
	before := buf.String()
	if err := pw.Flush(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != before {
		t.Fatalf("flush after newline modified buffer: %q -> %q", before, buf.String())
	}
}

func TestPrefixWriterEmbeddedNewlinesInOneWrite(t *testing.T) {
	var buf bytes.Buffer
	pw := newPrefixWriter("> ", &buf)
	// Worst case: arbitrary chunk boundaries with multiple newlines and a partial.
	chunks := []string{"line1\nlin", "e2\nline3", "\npartial"}
	for _, c := range chunks {
		if _, err := pw.Write([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	if err := pw.Flush(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	wantLines := []string{
		"> line1",
		"> line2",
		"> line3",
		"> partial",
		"",
	}
	if got != strings.Join(wantLines, "\n") {
		t.Fatalf("got %q", got)
	}
}
