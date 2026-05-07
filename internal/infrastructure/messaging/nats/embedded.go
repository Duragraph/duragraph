// Package nats — embedded.go
//
// Thin wrapper around `github.com/nats-io/nats-server/v2/server` that
// runs a real NATS server (with JetStream enabled) in-process as a
// goroutine of the duragraph engine. Used by the "embedded" binary
// mode (binary-modes.yml § embedded_components.nats_jetstream).
//
// The wrapper exists for two reasons:
//
//  1. Keep the upstream `server.Options` shape away from the cmd/serve.go
//     wiring — serve.go consumes a flat EmbeddedConfig populated from
//     internal/config.NATSConfig, not the 100+ field upstream struct.
//  2. Give a context-friendly Start/Stop signature consistent with the
//     embedded postgres wrapper. Upstream's `Server.Start()` is a
//     blocking goroutine entry; `ReadyForConnections(timeout)` is the
//     readiness gate. We hide that boilerplate from callers.
//
// Lifecycle expectations (per spec):
//   - Start launches the NATS server in a goroutine and blocks until
//     it is accepting client connections (ReadyForConnections returns
//     true). On timeout we return an error and tear the server down so
//     the caller doesn't have to deal with a half-started server.
//   - Stop calls Shutdown() then WaitForShutdown() — symmetric with
//     postgres.EmbeddedPostgres.Stop, which similarly blocks until the
//     subprocess exits cleanly.
//   - The data directory (cfg.DataDir) is preserved across Start/Stop
//     pairs. JetStream stores its streams there; durability across
//     restarts is the whole point of the embedded mode.
//
// The package consumer wiring (publisher.go, subscriber.go, task_queue.go,
// jetstream_subscriber.go) is unchanged — they read cfg.NATS.URL which
// internal/config.Load() already forced to point at the embedded server.
// No per-call-site mode checks anywhere downstream.
package nats

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// EmbeddedConfig is the flat config struct populated by serve.go from
// internal/config.NATSConfig. Fields mirror the relevant subset of
// server.Options; everything else (clustering, leaf nodes, MQTT, WS,
// etc.) is intentionally absent — embedded mode targets single-node
// JetStream, not distributed deployments.
type EmbeddedConfig struct {
	// Port is the client-facing TCP port (default 4222 in spec).
	Port int

	// DataDir is the JetStream store directory. Persistent across
	// restarts; created with 0o700 perms by the wrapper if absent.
	DataDir string

	// MonitorPort enables the HTTP /varz monitoring endpoint when
	// non-zero. 0 means disabled (spec default — the monitoring port
	// is a security surface, opt-in only).
	MonitorPort int

	// StartTimeout caps how long Start() waits for the server to
	// reach ReadyForConnections. Zero means "use library default"
	// (10 seconds, matching upstream's RunServerCallback in
	// nats-server/test/test.go).
	StartTimeout time.Duration
}

const defaultEmbeddedStartTimeout = 10 * time.Second

// EmbeddedNATS wraps the upstream *server.Server so the surrounding
// code can drive Start/Stop with context semantics consistent with
// the rest of the codebase.
type EmbeddedNATS struct {
	inner *server.Server
	cfg   EmbeddedConfig
}

// NewEmbedded constructs an EmbeddedNATS ready to Start. We create
// cfg.DataDir up front (with restrictive 0o700 perms — JetStream
// stores credentials and persisted streams there) so the upstream
// library doesn't have to recurse parent paths itself.
func NewEmbedded(cfg EmbeddedConfig) (*EmbeddedNATS, error) {
	if cfg.Port == 0 {
		return nil, fmt.Errorf("embedded nats: port is required")
	}
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("embedded nats: data directory is required")
	}

	// Ensure DataDir exists with restrictive perms. JetStream's store
	// directory holds stream data; group/world-readable is a leak for
	// any payload that flows through (RUNS, EXECUTION, etc.).
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("embedded nats: create data dir %s: %w", cfg.DataDir, err)
	}

	// MkdirAll does not chmod a pre-existing dir; tighten perms in case
	// the dir was created by an earlier (looser) version of this code
	// or by an operator's manual `mkdir`. Non-fatal — failure here
	// just means an upgrading operator's existing dir keeps its old
	// perms.
	if err := os.Chmod(cfg.DataDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr,
			"warning: embedded nats: chmod 0700 %s failed: %v (continuing)\n",
			cfg.DataDir, err)
	}

	opts := &server.Options{
		// Bind 127.0.0.1 only — embedded mode is a single-process
		// deployment, exposing 0.0.0.0 would create a listener that
		// looks identical to a properly hardened external NATS while
		// silently lacking auth.
		Host: "127.0.0.1",
		Port: cfg.Port,

		// JetStream is the whole reason we run NATS at all (durable
		// streams for runs/execution/interrupts/tasks). The zero
		// values for JetStreamMaxMemory / JetStreamMaxStore mean
		// "library defaults" — sized off available system memory and
		// the StoreDir's filesystem free space respectively.
		JetStream: true,
		StoreDir:  cfg.DataDir,

		// Engine owns signal handling. Without NoSigs=true, NATS
		// would install its own SIGINT/SIGTERM handler that races
		// with cmd/serve.go's signal.Notify and can swallow the
		// shutdown signal entirely. Mirrors what cmd/nats-server
		// does NOT do — but the embedded use case is the inverse of
		// the binary one.
		NoSigs: true,

		// Default-quiet logging. Operators wanting verbose NATS logs
		// can set the engine's own log level — wiring NATS's logger
		// interface to the engine logger is a follow-up (the upstream
		// type is server.Logger interface, not io.Writer, so a clean
		// adapter is its own small file).
		NoLog: false,
		Debug: false,
		Trace: false,
	}

	if cfg.MonitorPort > 0 {
		opts.HTTPHost = "127.0.0.1"
		opts.HTTPPort = cfg.MonitorPort
	}

	s, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("embedded nats: new server: %w", err)
	}

	return &EmbeddedNATS{
		inner: s,
		cfg:   cfg,
	}, nil
}

// Start launches the NATS server in a goroutine and blocks until it
// is accepting client connections. If the readiness check times out
// we Shutdown the server before returning, so the caller is never
// left holding a half-started reference.
//
// The ctx parameter is honoured for cancellation symmetry with the
// rest of the codebase — upstream's Start() / ReadyForConnections()
// don't accept a context, but if ctx is already done at call time we
// short-circuit with ctx.Err().
func (e *EmbeddedNATS) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	timeout := e.cfg.StartTimeout
	if timeout <= 0 {
		timeout = defaultEmbeddedStartTimeout
	}

	// server.Server.Start() blocks running the accept loop, so it has
	// to live in its own goroutine. This mirrors upstream's
	// RunServerCallback in nats-server/test/test.go — the canonical
	// in-process embedding pattern.
	go e.inner.Start()

	if !e.inner.ReadyForConnections(timeout) {
		// Tear down the half-started server so the caller doesn't
		// leak it. WaitForShutdown is bounded by the upstream
		// Shutdown sequence (max ~10s in pathological cases).
		e.inner.Shutdown()
		e.inner.WaitForShutdown()
		return fmt.Errorf("embedded nats: not ready for connections within %s", timeout)
	}

	return nil
}

// Stop initiates a graceful shutdown of the NATS server and waits
// for the server goroutine to exit. The ctx parameter is honoured
// the same way as in Start — short-circuit if already cancelled.
//
// We deliberately call WaitForShutdown unconditionally (not gated on
// ctx) once Shutdown is in flight: the upstream sequence is
// fsync-then-close-then-return, and abandoning halfway through
// invites corruption of JetStream's filestore.
func (e *EmbeddedNATS) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	e.inner.Shutdown()
	e.inner.WaitForShutdown()
	return nil
}

// ClientURL returns the client connection URL the embedded server is
// listening on (e.g. "nats://127.0.0.1:4222"). Useful for tests and
// for confirming the resolved address in startup logging — production
// callers go through cfg.NATS.URL which Load() forced to the same
// value.
func (e *EmbeddedNATS) ClientURL() string {
	return e.inner.ClientURL()
}

// Config returns the resolved EmbeddedConfig. Mostly useful for tests
// and for the loud-startup logging in serve.go.
func (e *EmbeddedNATS) Config() EmbeddedConfig {
	return e.cfg
}
