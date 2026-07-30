// Package server is the Layer-4 composition root of the control-plane
// rebuild per STRUCTURE.md. It wires the migrations, the
// endpoints.Server (tenant + platform pgxpool), the NATS relay, and
// the Echo router into a single runnable binary with graceful shutdown.
//
// Source of truth for the assembly: spec/models/system-architecture.d2
// (api → endpoints → relay → nats, plus the platform-provisioner side
// effect) and spec/models/d2/relay.d2 + nats.d2. The d2 stays the
// human spec — this file is its machine form.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/duragraph/duragraph/controlplane/endpoints"
	"github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/reaper"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// Defaults exported for callers/tests that want the same constants.
const (
	DefaultAddr         = ":8081"
	DefaultDrainTimeout = 15 * time.Second
	DefaultMigrateDir   = "controlplane/db/migrations"
)

// Config carries the runtime knobs the composition root needs. Zero
// values are tolerated for the optional ones; required fields are
// validated in New.
type Config struct {
	// Addr is the listen address. Default ":8081".
	Addr string

	// TenantDSN is the Postgres DSN for the per-tenant database the
	// generated endpoints read/write (assistants, threads, runs, crons,
	// store, workers, event store, outbox). Required.
	TenantDSN string

	// PlatformDSN is the Postgres DSN for the shared platform database
	// (users, tenants). Optional — without it the auth/admin/platform
	// groups return 500, but the rest still works for a single-tenant
	// bootstrap.
	PlatformDSN string

	// NATSURL is the JetStream URL for the outbox relay + SSE
	// subscriber. Empty disables the relay (read endpoints still work).
	NATSURL string

	// ListenerDSN is the bare Postgres DSN used for LISTEN outbox_new.
	// Must NOT go through PgBouncer (session affinity required). If
	// empty, falls back to TenantDSN — fine for dev/test, wrong for a
	// pooler-fronted production deploy.
	ListenerDSN string

	// MigrateDir is the parent dir holding tenant/ + platform/
	// subdirectories of *.up.sql migrations. Default
	// controlplane/db/migrations (resolved relative to CWD at runtime).
	MigrateDir string

	// Migrate controls whether ApplyMigrations runs on startup. Default
	// true; set false where migrations are owned by an external process.
	Migrate bool

	// Relays controls whether the outbox relay + cleanup worker start.
	// Default false; set true when NATSURL is non-empty to enable
	// event publishing.
	Relays bool

	// DrainTimeout caps how long Shutdown waits for in-flight HTTP
	// requests + relay goroutines to drain. Default 15s.
	DrainTimeout time.Duration
}

func (c *Config) defaults() {
	if c.Addr == "" {
		c.Addr = DefaultAddr
	}
	if c.DrainTimeout == 0 {
		c.DrainTimeout = DefaultDrainTimeout
	}
	if c.MigrateDir == "" {
		c.MigrateDir = DefaultMigrateDir
	}
}

// Server is the assembled control-plane: pools opened, endpoints
// mounted on Echo, relay goroutines running. Run blocks until a
// SIGINT/SIGTERM arrives or the context cancels; Shutdown drains.
type Server struct {
	cfg          Config
	tenant       *pgxpool.Pool
	plat         *pgxpool.Pool
	relay        *nats.Relay
	cleanup      *nats.CleanupWorker
	runProcessor *nats.RunProcessor
	reaper       *reaper.RunReaper
	echo         *echo.Echo

	relayDone   chan error
	cleanupDone chan error
	rpDone      chan error
	reaperDone  chan error

	closeOnce sync.Once
}

// New validates Config, opens the pgxpools, applies migrations (if
// cfg.Migrate), connects to NATS + ensures streams/consumers (if
// cfg.NATSURL), constructs the outbox relay + cleanup worker, and
// mounts all 10 generated endpoint groups on a fresh Echo. Returns a
// Server ready to Run. Pool + NATS connection ownership transfers to
// the returned Server; Close releases them.
func New(ctx context.Context, cfg Config) (*Server, error) {
	cfg.defaults()
	if cfg.TenantDSN == "" {
		return nil, errors.New("server: TenantDSN is required")
	}

	s := &Server{
		cfg:         cfg,
		relayDone:   make(chan error, 1),
		cleanupDone: make(chan error, 1),
		rpDone:      make(chan error, 1),
		reaperDone:  make(chan error, 1),
	}

	// --- pgxpools ---
	tenantPool, err := pgxpool.New(ctx, cfg.TenantDSN)
	if err != nil {
		return nil, fmt.Errorf("server: tenant pool: %w", err)
	}
	s.tenant = tenantPool
	if cfg.PlatformDSN != "" {
		platformPool, err := pgxpool.New(ctx, cfg.PlatformDSN)
		if err != nil {
			tenantPool.Close()
			return nil, fmt.Errorf("server: platform pool: %w", err)
		}
		s.plat = platformPool
	}

	// --- run reaper (needs only the tenant pool, not NATS) ---
	if s.tenant != nil {
		s.reaper = reaper.NewRunReaper(s.tenant, reaper.Config{})
	}

	// --- migrations ---
	if cfg.Migrate {
		tenantDir := filepath.Join(cfg.MigrateDir, "tenant")
		if err := ApplyMigrations(ctx, s.tenant, tenantDir); err != nil {
			s.Close()
			return nil, fmt.Errorf("server: tenant migrations: %w", err)
		}
		platDir := filepath.Join(cfg.MigrateDir, "platform")
		if _, err := os.Stat(platDir); err == nil && s.plat != nil {
			if err := ApplyMigrations(ctx, s.plat, platDir); err != nil {
				s.Close()
				return nil, fmt.Errorf("server: platform migrations: %w", err)
			}
		}
	}

	// --- NATS relay + cleanup worker (optional) ---
	if cfg.NATSURL != "" {
		nc, js, err := nats.Connect(ctx, cfg.NATSURL)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("server: nats connect: %w", err)
		}
		// EnsureConsumers is a separate call so test deployments that
		// want streams only can skip it. Production wants both — call
		// here for the default. Idempotent.
		if err := nats.EnsureConsumers(ctx, js); err != nil {
			nc.Drain()
			s.Close()
			return nil, fmt.Errorf("server: nats ensure consumers: %w", err)
		}
		publisher := nats.NewPublisher(js)
		listenerDSN := cfg.ListenerDSN
		if listenerDSN == "" {
			listenerDSN = cfg.TenantDSN
		}
		s.relay = nats.NewRelay(
			nats.NewOutboxDrain(s.tenant),
			publisher,
			listenerDSN,
			nats.DefaultSafetyNet,
			nats.DefaultBatchSize,
		)
		s.cleanup = nats.NewCleanupWorker(
			nats.NewOutboxDrain(s.tenant),
			time.Hour,
			7, // 7-day retention per CLAUDE.md default
		)
		// run-processor: dispatches run.created (via the relay above) as
		// worker.graph.execute commands, enriched from the tenant pool. See
		// controlplane/nats/run_processor.go.
		s.runProcessor = nats.NewRunProcessor(js, publisher, s.tenant)
		// nc lives for the relay's lifetime; closed on Shutdown via
		// the relay's Stop → publisher Drain. For the SSE subscriber,
		// there's a separate NewSubscriberFromConn path (TODO when
		// SSE handlers land).
	}

	// --- Echo router with all 10 endpoint groups mounted ---
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	// Cap idle keep-alive connections so Shutdown doesn't hang waiting
	// for clients that took the connection and went quiet. 60s
	// covers a slow curl pipe without holding a conn forever.
	e.Server.IdleTimeout = 60 * time.Second
	s.echo = e

	ep := &endpoints.Server{
		Tenant:   s.tenant,
		Platform: s.plat,
	}
	g := e.Group("/api/v1")
	ep.RegisterAssistants(g)
	ep.RegisterThreads(g)
	ep.RegisterRuns(g)
	ep.RegisterCrons(g)
	ep.RegisterStore(g)
	ep.RegisterWorkers(g)
	ep.RegisterAuth(g)
	ep.RegisterPlatform(g)
	ep.RegisterAdmin(g)
	ep.RegisterSystem(e) // root-level: /ok, /info, /metrics

	return s, nil
}

// Run blocks until ctx is canceled or a SIGINT/SIGTERM arrives, then
// calls Shutdown. Returns nil on a clean shutdown, ctx.Err() if the
// context canceled, or the listen error if the HTTP server exited
// outside a signal-driven shutdown.
func (s *Server) Run(ctx context.Context) error {
	// --- relay ---
	if s.relay != nil && s.cfg.Relays {
		go func() { s.relayDone <- s.relay.Start(ctx) }()
		if s.cleanup != nil {
			go func() { s.cleanupDone <- s.cleanup.Start(ctx) }()
		}
	}
	if s.runProcessor != nil && s.cfg.Relays {
		go func() { s.rpDone <- s.runProcessor.Start(ctx) }()
	}
	if s.reaper != nil && s.cfg.Relays {
		go func() { s.reaperDone <- s.reaper.Start(ctx) }()
	}

	// --- HTTP ---
	httpErr := make(chan error, 1)
	go func() {
		slog.Info("duragraph server listening", "addr", s.cfg.Addr)
		if err := s.echo.Start(s.cfg.Addr); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			httpErr <- err
		}
	}()

	// --- wait for signal / ctx / http error ---
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-ctx.Done():
		slog.Info("duragraph server: context canceled, shutting down")
	case sig := <-sig:
		slog.Info("duragraph server: signal received, shutting down", "sig", sig.String())
	case err := <-httpErr:
		slog.Error("duragraph server: http listener exited", "err", err)
		return err
	}
	return s.Shutdown(ctx)
}

// Shutdown drains everything in the right order: HTTP stops accepting
// connections, in-flight requests drain up to DrainTimeout, relay +
// cleanup goroutines stop, pools close. Idempotent.
func (s *Server) Shutdown(ctx context.Context) error {
	shutCtx, cancel := context.WithTimeout(ctx, s.cfg.DrainTimeout)
	defer cancel()

	// Stop the HTTP listener so no new requests arrive.
	if s.echo != nil {
		_ = s.echo.Shutdown(shutCtx)
	}
	// Stop the relay + cleanup so they don't drain against the pools
	// we're about to close.
	if s.relay != nil {
		s.relay.Stop()
	}
	if s.cleanup != nil {
		s.cleanup.Stop()
	}
	if s.runProcessor != nil {
		s.runProcessor.Stop()
	}
	if s.reaper != nil {
		s.reaper.Stop()
	}
	// Wait for those goroutines to exit (bounded by DrainTimeout).
	select {
	case <-s.relayDone:
	case <-shutCtx.Done():
	}
	if s.cleanup != nil {
		select {
		case <-s.cleanupDone:
		case <-shutCtx.Done():
		}
	}
	if s.runProcessor != nil {
		select {
		case <-s.rpDone:
		case <-shutCtx.Done():
		}
	}
	if s.reaper != nil {
		select {
		case <-s.reaperDone:
		case <-shutCtx.Done():
		}
	}
	s.Close()
	return nil
}

// Close releases the pgxpools without attempting graceful drain of
// in-flight requests or relay goroutines. Safe to call multiple times.
// Used as the defer-cleanup path in New on early errors + as the
// final step of Shutdown.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		if s.tenant != nil {
			s.tenant.Close()
		}
		if s.plat != nil {
			s.plat.Close()
		}
	})
}
