package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSetupWith_JSON_emitsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	SetupWith(&buf, FormatJSON, LevelInfo)
	slog.Info("hello world", "run_id", "r-123", "count", 7)

	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected JSON line, got %q", out)
	}
	for _, want := range []string{`"msg":"hello world"`, `"run_id":"r-123"`, `"count":7`, `"level":"INFO"`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON output missing %s\ngot: %s", want, out)
		}
	}
}

func TestSetupWith_Pretty_writesHumanReadable(t *testing.T) {
	var buf bytes.Buffer
	SetupWith(&buf, FormatPretty, LevelInfo)
	slog.Info("hello world", "run_id", "r-123")

	out := buf.String()
	if !strings.Contains(out, "hello world") {
		t.Errorf("pretty output missing message: %q", out)
	}
	if !strings.Contains(out, "run_id") {
		t.Errorf("pretty output missing key: %q", out)
	}
	// Pretty output should NOT start with `{` — that would mean it
	// fell through to the JSON handler by mistake.
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("pretty output looks like JSON: %q", out)
	}
}

func TestResolveFormat_AutoNonTTYIsJSON(t *testing.T) {
	// A *bytes.Buffer is not an *os.File → not a TTY → JSON.
	got := resolveFormat(&bytes.Buffer{}, FormatAuto)
	if got != FormatJSON {
		t.Errorf("Auto on non-TTY should be JSON, got %v", got)
	}
}

func TestResolveFormat_ExplicitNotOverridden(t *testing.T) {
	if got := resolveFormat(&bytes.Buffer{}, FormatPretty); got != FormatPretty {
		t.Errorf("Pretty should pass through, got %v", got)
	}
	if got := resolveFormat(&bytes.Buffer{}, FormatJSON); got != FormatJSON {
		t.Errorf("JSON should pass through, got %v", got)
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]Format{
		"json":    FormatJSON,
		"JSON":    FormatJSON,
		"pretty":  FormatPretty,
		"text":    FormatPretty,
		"console": FormatPretty,
		"":        FormatAuto,
		"garbage": FormatAuto,
	}
	for in, want := range cases {
		if got := ParseFormat(in); got != want {
			t.Errorf("ParseFormat(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseLevelString(t *testing.T) {
	cases := map[string]Level{
		"debug":   LevelDebug,
		"DEBUG":   LevelDebug,
		"info":    LevelInfo,
		"":        LevelInfo,
		"warn":    LevelWarn,
		"warning": LevelWarn,
		"error":   LevelError,
		"err":     LevelError,
	}
	for in, want := range cases {
		if got := ParseLevelString(in); got != want {
			t.Errorf("ParseLevelString(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSetupWith_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	SetupWith(&buf, FormatJSON, LevelWarn)
	slog.Debug("debug should be filtered")
	slog.Info("info should be filtered")
	slog.Warn("warn should pass")
	slog.Error("error should pass")

	out := buf.String()
	if strings.Contains(out, "debug should be filtered") || strings.Contains(out, "info should be filtered") {
		t.Errorf("level filter let through low-priority log:\n%s", out)
	}
	if !strings.Contains(out, "warn should pass") || !strings.Contains(out, "error should pass") {
		t.Errorf("level filter dropped warn/error:\n%s", out)
	}
}
