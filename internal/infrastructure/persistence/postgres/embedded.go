// Package postgres — embedded.go
//
// Thin wrapper around fergusstrange/embedded-postgres that spawns a real
// postgres process as a child of the duragraph engine. Used by the
// "embedded" binary mode (binary-modes.yml § embedded_components.postgres).
//
// The wrapper exists for three reasons:
//
//  1. Keep the library's fluent-builder API away from the cmd/serve.go
//     wiring — serve.go consumes a flat EmbeddedConfig struct populated
//     from internal/config, not a chain of method calls.
//  2. Map the config package's plain string version ("15") to the
//     library's typed embeddedpostgres.PostgresVersion. This isolates
//     the heavy library dep from the config package.
//  3. Give us a stable seam for future test substitution (interface
//     extraction is deferred until we actually need it — Phase 4).
//
// Lifecycle expectations (per spec):
//   - Start blocks until the postgres process is accepting connections.
//   - Stop sends SIGTERM equivalent and waits for graceful shutdown
//     (the library handles the fsync/checkpoint/exit dance).
//   - The data directory (cfg.DataDir) is preserved across Start/Stop
//     pairs. Runtime path and binary cache use the library defaults
//     (~/.embedded-postgres-go/) so first start fetches the binary;
//     subsequent starts reuse the cache.
package postgres

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

// EmbeddedConfig is the flat config struct populated by serve.go from
// internal/config.DatabaseConfig. Fields mirror the library's relevant
// builder methods 1:1.
type EmbeddedConfig struct {
	Port     uint32
	DataDir  string // persistent data path, NOT the runtime path
	Username string
	Password string
	Database string
	Version  string // postgres major version string, e.g. "15"
	Logger   io.Writer

	// StartTimeout caps how long Start() waits for the postgres process
	// to become healthy. Zero means "use library default" (15s, which is
	// enough for cached binaries but tight for a first-run download on
	// slow links — operators can bump via env var if needed).
	StartTimeout time.Duration
}

// EmbeddedPostgres wraps the library's *EmbeddedPostgres so we can give
// the surrounding code a context-friendly Start/Stop signature without
// fighting the library's blocking calls.
type EmbeddedPostgres struct {
	inner *embeddedpostgres.EmbeddedPostgres
	cfg   EmbeddedConfig
}

// resolvePostgresVersion maps the major-version string from
// EmbeddedConfig.Version to the library's PostgresVersion constant.
// Defaults to V15 (matches prod-postgres) when the input is empty or
// unrecognised — emit no error here because the library itself accepts
// arbitrary semver strings as a fallback path.
func resolvePostgresVersion(version string) embeddedpostgres.PostgresVersion {
	switch version {
	case "", "15":
		return embeddedpostgres.V15
	case "16":
		return embeddedpostgres.V16
	case "17":
		return embeddedpostgres.V17
	case "18":
		return embeddedpostgres.V18
	case "14":
		return embeddedpostgres.V14
	case "13":
		return embeddedpostgres.V13
	case "12":
		return embeddedpostgres.V12
	default:
		// Pass through as a custom semver — embeddedpostgres.PostgresVersion
		// is a string newtype, so this works for e.g. "15.3.0".
		return embeddedpostgres.PostgresVersion(version)
	}
}

// NewEmbedded constructs an EmbeddedPostgres ready to Start. The data
// directory is created by the library on first start (initdb), so the
// caller does NOT need to mkdir it ahead of time — but we DO ensure the
// parent exists, since the library does not recursively create the
// runtime path's parents.
func NewEmbedded(cfg EmbeddedConfig) (*EmbeddedPostgres, error) {
	if cfg.Port == 0 {
		return nil, fmt.Errorf("embedded postgres: port is required")
	}
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("embedded postgres: data directory is required")
	}
	if cfg.Username == "" || cfg.Password == "" || cfg.Database == "" {
		return nil, fmt.Errorf("embedded postgres: username, password and database are required")
	}

	// Ensure the data directory's parent exists. The library mkdirs the
	// data dir itself (initdb creates it) but won't create parents.
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("embedded postgres: create data dir parent: %w", err)
	}

	builder := embeddedpostgres.DefaultConfig().
		Version(resolvePostgresVersion(cfg.Version)).
		Port(cfg.Port).
		Username(cfg.Username).
		Password(cfg.Password).
		Database(cfg.Database).
		// DataPath is the persistent dir — preserved across restarts.
		// We deliberately do NOT set RuntimePath because the library
		// erases it on every Start(). Default runtime path lives under
		// $TMPDIR/embedded-postgres-go/<version> which is fine for the
		// binary extraction step.
		DataPath(cfg.DataDir)

	if cfg.StartTimeout > 0 {
		builder = builder.StartTimeout(cfg.StartTimeout)
	}
	if cfg.Logger != nil {
		builder = builder.Logger(cfg.Logger)
	}

	return &EmbeddedPostgres{
		inner: embeddedpostgres.NewDatabase(builder),
		cfg:   cfg,
	}, nil
}

// Start launches the embedded postgres process and blocks until it is
// accepting connections. The ctx parameter is currently honoured only
// for symmetry with the rest of the codebase — the library's own
// Start() does not take a context. If the caller's ctx is already done
// when Start is invoked we short-circuit with ctx.Err().
func (e *EmbeddedPostgres) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := e.inner.Start(); err != nil {
		return fmt.Errorf("embedded postgres start: %w", err)
	}
	return nil
}

// Stop gracefully terminates the embedded postgres process. The
// library's Stop() blocks until pg_ctl reports a clean shutdown
// (fsync + checkpoint + exit). The ctx parameter is honoured the same
// way as in Start — short-circuit if already cancelled.
func (e *EmbeddedPostgres) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := e.inner.Stop(); err != nil {
		return fmt.Errorf("embedded postgres stop: %w", err)
	}
	return nil
}

// Config returns the resolved EmbeddedConfig. Mostly useful for tests
// and for the loud-startup logging in serve.go.
func (e *EmbeddedPostgres) Config() EmbeddedConfig {
	return e.cfg
}
