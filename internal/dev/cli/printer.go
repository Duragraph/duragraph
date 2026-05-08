// Package cli holds the client-side helpers used by the duragraph CLI
// subcommands (`runs *`, `events tail`). These commands talk over the
// network to a running engine — they don't embed engine internals — so
// the helpers here are deliberately thin: an HTTP wrapper, a SSE line
// reader, and a NATS event decoder.
//
// Implements binary-modes.yml § subcommands.duragraph_runs and
// § subcommands.duragraph_events (v0.7 Phase 7 of the single-binary DX
// track).
package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// PrintJSON pretty-prints v as indented JSON followed by a newline.
//
// Output is monochrome by design: the project does not currently depend
// on any ANSI color library, and Phase 7 is not the right place to
// introduce one. If a future PR wires a logger with color, the right
// move is to swap fmt.Fprintln here for that logger's structured
// printer, not to bolt a second color dep onto the binary.
//
// Errors from json.MarshalIndent are surfaced rather than swallowed —
// the CLI is a thin wrapper over arbitrary engine responses, and a
// marshal failure usually indicates an unexpected payload shape that
// the operator needs to see.
func PrintJSON(w io.Writer, v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if _, err := fmt.Fprintln(w, string(out)); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

// PrintEvent prints a single SSE / NATS event with a leading event-type
// prefix line, then the payload as indented JSON. Used by both `runs
// tail` and `events tail` so their on-screen format is identical.
//
// The prefix line is a comment-style marker (// event_type) that grep
// can match on but a JSON parser will skip — operators piping to `jq`
// commonly want the JSON portion only, which they get with
// `grep -v '^// '`.
func PrintEvent(w io.Writer, eventType string, payload any) error {
	if eventType != "" {
		if _, err := fmt.Fprintf(w, "// %s\n", eventType); err != nil {
			return fmt.Errorf("write event-type prefix: %w", err)
		}
	}
	return PrintJSON(w, payload)
}
