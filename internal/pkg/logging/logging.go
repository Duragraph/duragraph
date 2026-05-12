// Package logging configures slog.Default() so the rest of the codebase
// can call slog.{Info,Warn,Error,Debug} without thinking about output
// format. Two handler shapes:
//
//   - "pretty": charmbracelet/log adapted as a slog.Handler. Colorised,
//     timestamped, key=value attribute rendering. Right for `duragraph
//     dev` and any TTY-backed run.
//   - "json":   stdlib slog.NewJSONHandler. Structured one-object-per-
//     line output. Right for production containers — Loki / Vector /
//     the OTel collector parse it without any agent-side regex glue.
//
// Format is chosen explicitly (--log-format flag or DURAGRAPH_LOG_FORMAT
// env var) or auto-detected: if stderr is a TTY, pretty; otherwise JSON.
// Level is "info" by default; --log-level / DURAGRAPH_LOG_LEVEL accept
// debug | info | warn | error.
//
// Setup() is idempotent and safe to call multiple times — useful when
// the cobra root sets a default early and a subcommand wants to override
// after parsing its own flags.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	charmlog "github.com/charmbracelet/log"
	"github.com/mattn/go-isatty"
)

// Format selects the slog.Handler shape.
type Format string

const (
	// FormatAuto picks Pretty when stderr is a TTY, JSON otherwise.
	// The default — covers laptop dev (pretty) and container prod
	// (JSON) without anyone having to think about it.
	FormatAuto Format = "auto"
	// FormatPretty forces charmbracelet/log output regardless of TTY.
	FormatPretty Format = "pretty"
	// FormatJSON forces stdlib JSON output regardless of TTY.
	FormatJSON Format = "json"
)

// Level wraps the four slog levels we expose through flags.
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Setup configures slog.Default() with a handler matching format/level.
// Output goes to stderr (stdout is reserved for protocol output like
// the engine's startup banner and JSON-on-stdout subcommands).
func Setup(format Format, level Level) {
	slog.SetDefault(slog.New(newHandler(os.Stderr, format, level)))
}

// SetupWith is the dependency-injectable form of Setup — useful for
// tests that want to capture log output to a bytes.Buffer.
func SetupWith(w io.Writer, format Format, level Level) {
	slog.SetDefault(slog.New(newHandler(w, format, level)))
}

func newHandler(w io.Writer, format Format, level Level) slog.Handler {
	lvl := parseLevel(level)
	switch resolveFormat(w, format) {
	case FormatPretty:
		// charmbracelet/log implements slog.Handler since v0.4. We
		// don't expose ReportCaller because it's noisy for the kinds
		// of logs the engine emits (mostly lifecycle / event prints,
		// not deep call stacks). TimeFormat omitted -> charm's default
		// "15:04:05" short form, which fits dev-mode density.
		return charmlog.NewWithOptions(w, charmlog.Options{
			ReportTimestamp: true,
			Level:           charmlog.Level(lvl),
		})
	default: // FormatJSON
		return slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: lvl,
		})
	}
}

// resolveFormat translates Auto into Pretty or JSON based on whether
// the writer is a TTY. Non-os.File writers (bytes.Buffer in tests,
// piped output, redirected stderr) are treated as non-TTY → JSON.
func resolveFormat(w io.Writer, f Format) Format {
	if f != FormatAuto {
		return f
	}
	if file, ok := w.(*os.File); ok && isatty.IsTerminal(file.Fd()) {
		return FormatPretty
	}
	return FormatJSON
}

func parseLevel(l Level) slog.Level {
	switch strings.ToLower(string(l)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ParseFormat normalises whatever the user typed on the command line or
// passed via env var. Unknown values fall through to Auto rather than
// erroring — bad logging config should not abort the binary.
func ParseFormat(s string) Format {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pretty", "text", "console":
		return FormatPretty
	case "json":
		return FormatJSON
	default:
		return FormatAuto
	}
}

// ParseLevelString normalises level input. Falls through to Info.
func ParseLevelString(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "trace":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error", "err":
		return LevelError
	default:
		return LevelInfo
	}
}
