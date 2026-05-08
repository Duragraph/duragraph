package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duragraph/duragraph/internal/dev/cli"
	"github.com/spf13/cobra"
)

// Phase 7 (binary-modes.yml § subcommands.duragraph_runs) implementation.
//
// All four `runs *` subcommands are CLIENTS — they target a running
// `duragraph serve` (or `duragraph dev`) instance over the network.
// They never embed engine internals or open a database connection
// directly, so they remain safe to ship in a CI image without DB drivers.
//
// Configuration:
//   - DURAGRAPH_URL       — engine base URL (default http://localhost:8081),
//                           overridable per-invocation via --engine.
//   - DURAGRAPH_NATS_URL  — NATS URL for `runs tail` no-arg fallback
//                           (default nats://localhost:4222), overridable via --nats.
//
// Default behaviour rationale: NATS-subscribing for "all threads" tail
// rather than polling /api/v1/runs. The engine already publishes run
// lifecycle events to the `duragraph.runs.run.>` subject family for
// the SSE bridge — re-using that surface keeps latency near-zero and
// avoids a separate poll loop. Engine SSE has no "all threads"
// endpoint, so polling would be the only HTTP alternative.

// Per-command flags. Defined as package vars rather than closure-captured
// inside RunE so they show up in `--help` output the same way the rest
// of this package does.
var (
	runsEngineURL      string
	runsTriggerInput   string
	runsTriggerWait    bool
	runsTailNATSURL    string
	runsTriggerThread  string // optional thread_id override; usually let server allocate
	runsTriggerTimeout time.Duration
)

var runsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Inspect, stream, and trigger runs via the engine API",
	Long: `runs talks to a running duragraph engine over HTTP/SSE/NATS to inspect,
stream, and trigger workflow runs. The engine URL defaults to
http://localhost:8081 and is overridable via DURAGRAPH_URL or --engine.

This is a thin client — start the engine first with "duragraph serve"
or "duragraph dev" in another terminal.`,
}

var runsTailCmd = &cobra.Command{
	Use:   "tail [thread_id]",
	Short: "Stream new runs across all threads (or a single thread)",
	Long: `Live-tail run lifecycle events.

With a thread_id argument, subscribes to the engine's per-thread SSE
endpoint (/api/v1/threads/<thread_id>/stream). Without an argument,
subscribes directly to the NATS run-event subject — the engine has no
"all threads" SSE endpoint, so NATS is the only zero-latency option.

Press Ctrl+C to stop.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRunsTail,
}

var runsGetCmd = &cobra.Command{
	Use:   "get <run_id>",
	Short: "Print a run's current state and output as JSON",
	Long: `Fetch a single run by ID via GET /api/v1/runs/<run_id> and pretty-print
the response. Exits non-zero with a clear message if the run does not
exist.`,
	Args: cobra.ExactArgs(1),
	RunE: runRunsGet,
}

var runsTriggerCmd = &cobra.Command{
	Use:   "trigger <assistant_id>",
	Short: "Create a one-shot run against an assistant",
	Long: `Create a stateless run via POST /api/v1/runs and print the engine's
response (run_id, thread_id, status). The engine allocates an ephemeral
thread automatically; pass --thread to attach the run to a specific
thread instead.

With --wait, polls the run until it reaches a terminal state
(completed | failed | cancelled) and prints the final state. Without
--wait, returns immediately after the run is queued.

The --input flag must be a JSON object — it's validated client-side
before sending so a typo doesn't cost a round-trip.`,
	Args: cobra.ExactArgs(1),
	RunE: runRunsTrigger,
}

func init() {
	runsCmd.PersistentFlags().StringVar(&runsEngineURL, "engine", "",
		"Engine base URL (defaults to $DURAGRAPH_URL or "+cli.DefaultEngineURL+")")

	runsTailCmd.Flags().StringVar(&runsTailNATSURL, "nats", "",
		"NATS URL for no-arg tail (defaults to $DURAGRAPH_NATS_URL or "+cli.DefaultNATSURL+")")

	runsTriggerCmd.Flags().StringVar(&runsTriggerInput, "input", "{}",
		"Run input as a JSON object (default {})")
	runsTriggerCmd.Flags().BoolVar(&runsTriggerWait, "wait", false,
		"Block until the run reaches a terminal state and print the final state")
	runsTriggerCmd.Flags().StringVar(&runsTriggerThread, "thread", "",
		"Optional existing thread_id; if empty the engine allocates an ephemeral one")
	runsTriggerCmd.Flags().DurationVar(&runsTriggerTimeout, "timeout", 5*time.Minute,
		"Maximum duration to wait when --wait is set")

	runsCmd.AddCommand(runsTailCmd, runsGetCmd, runsTriggerCmd)
	rootCmd.AddCommand(runsCmd)
}

// resolveEngineURL picks the engine URL with the documented precedence:
// --engine flag > $DURAGRAPH_URL > built-in default. Centralised so each
// subcommand RunE doesn't reproduce the same lookup.
func resolveEngineURL() string {
	if runsEngineURL != "" {
		return runsEngineURL
	}
	if v := os.Getenv("DURAGRAPH_URL"); v != "" {
		return v
	}
	return cli.DefaultEngineURL
}

func resolveNATSURL(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("DURAGRAPH_NATS_URL"); v != "" {
		return v
	}
	return cli.DefaultNATSURL
}

// runRunsGet implements `duragraph runs get <run_id>`.
func runRunsGet(cmd *cobra.Command, args []string) error {
	runID := args[0]
	c := cli.NewClient(resolveEngineURL())
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	body, err := c.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, cli.ErrRunNotFound) {
			return fmt.Errorf("run %s not found at %s", runID, c.EngineURL())
		}
		return err
	}
	// Re-decode → re-encode so the output is consistently indented
	// regardless of how the engine formatted its body.
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		// Fall back to printing raw — engine sent something we can't
		// decode but the operator deserves to see it.
		_, werr := fmt.Fprintln(cmd.OutOrStdout(), string(body))
		return werr
	}
	return cli.PrintJSON(cmd.OutOrStdout(), v)
}

// runRunsTrigger implements `duragraph runs trigger <assistant_id>`.
func runRunsTrigger(cmd *cobra.Command, args []string) error {
	assistantID := args[0]

	// Validate --input is a JSON object client-side. This catches the
	// common operator typo (single-quote vs double-quote, missing
	// braces) before the round-trip. Note: the engine's CreateRun
	// expects an object specifically (input fields are merged with
	// stream/interrupt config server-side), so a top-level JSON array
	// or scalar is rejected here even though it would be parseable.
	var input map[string]any
	if err := json.Unmarshal([]byte(runsTriggerInput), &input); err != nil {
		return fmt.Errorf("--input must be a JSON object: %w", err)
	}

	body := map[string]any{
		"assistant_id": assistantID,
		"input":        input,
	}
	if runsTriggerThread != "" {
		body["thread_id"] = runsTriggerThread
	}

	c := cli.NewClient(resolveEngineURL())
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	resp, err := c.CreateRun(ctx, body)
	if err != nil {
		return err
	}

	// Always emit the trigger response on stderr first so an operator
	// can see the run_id even if --wait is used and the long poll is
	// still in flight when they hit Ctrl+C.
	fmt.Fprintf(cmd.ErrOrStderr(), "queued run_id=%s thread_id=%s status=%s\n",
		resp.RunID, resp.ThreadID, resp.Status)

	if !runsTriggerWait {
		// Fire-and-forget mode: print structured response on stdout
		// for downstream tooling and exit.
		return cli.PrintJSON(cmd.OutOrStdout(), resp)
	}

	// --wait: long-poll the run until it terminates.
	waitCtx, waitCancel := context.WithTimeout(cmd.Context(), runsTriggerTimeout)
	defer waitCancel()

	final, err := c.WaitForRun(waitCtx, resp.RunID, time.Second)
	if err != nil {
		return fmt.Errorf("wait for run %s: %w", resp.RunID, err)
	}
	var v any
	if err := json.Unmarshal(final, &v); err != nil {
		_, werr := fmt.Fprintln(cmd.OutOrStdout(), string(final))
		return werr
	}
	return cli.PrintJSON(cmd.OutOrStdout(), v)
}

// runRunsTail implements `duragraph runs tail [thread_id]`.
func runRunsTail(cmd *cobra.Command, args []string) error {
	// Wire SIGINT/SIGTERM into the cobra context so Ctrl+C cleanly
	// drains the SSE / NATS subscription rather than dropping mid-frame.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	out := cmd.OutOrStdout()

	if len(args) == 1 {
		// Per-thread SSE path. Endpoint exists in the engine
		// (cmd/duragraph/cmd/serve.go: "/threads/:thread_id/stream").
		threadID := args[0]
		c := cli.NewClient(resolveEngineURL())
		fmt.Fprintf(cmd.ErrOrStderr(), "tailing thread=%s via SSE at %s\n", threadID, c.EngineURL())
		err := c.StreamRuns(ctx, threadID, func(ev cli.SSEEvent) error {
			var payload any
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				// Print raw on decode error.
				_, werr := fmt.Fprintf(out, "// %s\n%s\n", ev.Type, string(ev.Data))
				return werr
			}
			return cli.PrintEvent(out, ev.Type, payload)
		})
		// ctx-cancel produces a connection-closed error from Go's HTTP
		// stack rather than ctx.Err — treat that as clean exit.
		if err != nil && ctx.Err() == nil {
			return err
		}
		return nil
	}

	// No thread_id: subscribe directly to NATS for run lifecycle
	// events across all threads. Subject `duragraph.runs.run.>`
	// matches outbox_relay.buildTopic for aggregateType="run".
	natsURL := resolveNATSURL(runsTailNATSURL)
	const subject = "duragraph.runs.run.>"
	fmt.Fprintf(cmd.ErrOrStderr(), "tailing all runs via NATS %s subject=%s\n", natsURL, subject)
	err := cli.SubscribeEvents(ctx, natsURL, subject, "", func(env cli.EventEnvelope) error {
		return cli.PrintEvent(out, env.EventType, env)
	})
	if err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
