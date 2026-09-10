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
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	duragraph "github.com/duragraph/duragraph"
	"github.com/duragraph/duragraph/controlplane/cron"
	"github.com/duragraph/duragraph/controlplane/endpoints"
	"github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/reaper"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dashboard"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// Defaults exported for callers/tests that want the same constants.
const (
	DefaultAddr         = ":8081"
	DefaultDrainTimeout = 15 * time.Second
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

	// JWTSecret is the HMAC key for platform session tokens. Empty leaves
	// the platform surface (/api/auth, /api/platform, /api/admin) unable
	// to mint or verify sessions: auth answers 503 and /me answers 401,
	// rather than signing with an empty key that any other unconfigured
	// deployment could forge. Read from DURAGRAPH_JWT_SECRET when unset.
	JWTSecret []byte

	// BaseURL is the canonical external origin (scheme + host), used for
	// the logout CSRF origin check. Read from DURAGRAPH_BASE_URL.
	BaseURL string

	// CookieDomain scopes the session cookie; empty means host-only,
	// which is the correct default in dev. Read from
	// DURAGRAPH_COOKIE_DOMAIN.
	CookieDomain string

	// CookieSecure sets the Secure attribute on the session cookie. Must
	// be true wherever the platform is served over https. Read from
	// DURAGRAPH_COOKIE_SECURE.
	CookieSecure bool

	// NATSURL is the JetStream URL for the outbox relay + SSE
	// subscriber. Empty disables the relay (read endpoints still work).
	NATSURL string

	// ListenerDSN is the bare Postgres DSN used for LISTEN outbox_new.
	// Must NOT go through PgBouncer (session affinity required). If
	// empty, falls back to TenantDSN — fine for dev/test, wrong for a
	// pooler-fronted production deploy.
	ListenerDSN string

	// MigrateDir overrides where migrations are read from: a parent dir
	// holding tenant/ + platform/ subdirectories of *.up.sql files.
	//
	// EMPTY IS THE NORMAL CASE and means the migrations embedded in the
	// binary (controlplane/db). This used to default to a source-tree
	// path resolved against the process working directory, which meant an
	// installed binary could not migrate at all.
	MigrateDir string

	// Migrate controls whether ApplyMigrations runs on startup. Default
	// true; set false where migrations are owned by an external process.
	Migrate bool

	// Relays controls whether the outbox relay + cleanup worker start.
	// Default false; set true when NATSURL is non-empty to enable
	// event publishing.
	Relays bool

	// DashboardFS overrides the embedded React UI. Nil — the normal case —
	// serves the dashboard embedded in the binary. Supplying one lets a
	// deployment ship its own build, and lets tests assert routing without
	// depending on whether pnpm ran.
	DashboardFS fs.FS

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
	cronSched    *cron.Scheduler
	echo         *echo.Echo

	relayDone   chan error
	cleanupDone chan error
	rpDone      chan error
	reaperDone  chan error
	cronDone    chan error

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
		cronDone:    make(chan error, 1),
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

	// --- run reaper + cron scheduler (need only the tenant pool, not NATS) ---
	if s.tenant != nil {
		s.reaper = reaper.NewRunReaper(s.tenant, reaper.Config{})
		// Fires due crons by creating runs through the same event-sourced
		// path as the API, so a scheduled run is dispatched exactly like a
		// requested one.
		s.cronSched = cron.NewScheduler(s.tenant, cron.Config{})
	}

	// --- migrations ---
	if cfg.Migrate {
		root, err := migrationRoot(cfg.MigrateDir)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("server: migrations: %w", err)
		}
		tenantFS, err := fs.Sub(root, "tenant")
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("server: tenant migrations: %w", err)
		}
		if err := ApplyMigrationsFS(ctx, s.tenant, tenantFS); err != nil {
			s.Close()
			return nil, fmt.Errorf("server: tenant migrations: %w", err)
		}
		if platFS, err := fs.Sub(root, "platform"); err == nil && s.plat != nil {
			if err := ApplyMigrationsFS(ctx, s.plat, platFS); err != nil {
				s.Close()
				return nil, fmt.Errorf("server: platform migrations: %w", err)
			}
		}
	}

	// --- NATS relay + cleanup worker (optional) ---
	var subscriber *nats.Subscriber
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
		// the relay's Stop → publisher Drain. The SSE/wait endpoints
		// share the same conn via a Subscriber (core-NATS, no
		// separate connection or durable state).
		subscriber = nats.NewSubscriberFromConn(nc)
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

	// Environment fallbacks for the platform session settings. Config wins when
	// set explicitly (tests, embedding); the env vars are the deployment path.
	jwtSecret := cfg.JWTSecret
	if len(jwtSecret) == 0 {
		jwtSecret = []byte(os.Getenv("DURAGRAPH_JWT_SECRET"))
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("DURAGRAPH_BASE_URL")
	}
	cookieDomain := cfg.CookieDomain
	if cookieDomain == "" {
		cookieDomain = os.Getenv("DURAGRAPH_COOKIE_DOMAIN")
	}
	cookieSecure := cfg.CookieSecure
	if !cookieSecure {
		cookieSecure = os.Getenv("DURAGRAPH_COOKIE_SECURE") == "true"
	}
	if len(jwtSecret) == 0 {
		// Loud, because the failure mode is otherwise silent: every login
		// answers 503 and it looks like the provider is down.
		slog.Warn("platform session secret is not set; /api/auth, /api/platform and /api/admin " +
			"cannot establish sessions (set DURAGRAPH_JWT_SECRET)")
	}

	ep := &endpoints.Server{
		Tenant:     s.tenant,
		Platform:   s.plat,
		Subscriber: subscriber,
		Auth: endpoints.AuthConfig{
			JWTSecret:    jwtSecret,
			BaseURL:      baseURL,
			CookieDomain: cookieDomain,
			CookieSecure: cookieSecure,
		},
	}
	g := e.Group("/api/v1")
	ep.RegisterAssistants(g)
	ep.RegisterThreads(g)
	ep.RegisterRuns(g)
	ep.RegisterCrons(g)
	ep.RegisterStore(g)
	ep.RegisterWorkers(g)

	// The platform surface is NOT under /api/v1. endpoints.yaml declares these
	// with absolute paths (/api/auth/..., /api/platform/..., /api/admin/...)
	// because api.d2 puts them on their own surface — "Platform surface:
	// /api/auth/*, /api/platform/*, /api/admin/*". Mounting them on the v1
	// group concatenated the two and served /api/v1/api/admin/users, which is
	// in no diagram and no spec. They mount at the root instead.
	root := e.Group("")
	ep.RegisterAuth(root)
	ep.RegisterPlatform(root)
	ep.RegisterAdmin(root)

	ep.RegisterSystem(e) // root-level: /ok, /info, /metrics

	// The embedded React dashboard. Without it the rebuilt control plane
	// serves the API and nothing at "/" — the entire UI missing, which a
	// route-by-route diff against the legacy server cannot surface because
	// the dashboard is not a route.
	//
	// Mounted last to mirror the legacy server, but NOT because ordering is
	// load-bearing: Echo's radix router prefers static and parameterised
	// routes over a catch-all regardless of registration order, and mounting
	// this first leaves every API route reachable (verified). The legacy
	// comment claims otherwise; it is wrong on this point.
	//
	// What IS load-bearing is the /api/ guard inside the handler, which
	// turns an unmatched /api/... path into a clean 404 instead of serving
	// the SPA shell to a client expecting JSON.
	if err := mountDashboard(e, cfg.DashboardFS); err != nil {
		// A missing UI must not stop the control plane from serving the
		// API — an operator running headless is a legitimate deployment,
		// and failing here would turn a cosmetic problem into an outage.
		slog.Warn("dashboard not mounted; API-only", "err", err)
	}

	return s, nil
}

// mountDashboard serves the embedded UI, or the caller's filesystem when
// one is supplied (tests, and any deployment shipping its own build).
func mountDashboard(e *echo.Echo, override fs.FS) error {
	distFS := override
	if distFS == nil {
		var err error
		distFS, err = duragraph.DashboardFS()
		if err != nil {
			return fmt.Errorf("embedded dashboard: %w", err)
		}
	}
	// A placeholder index.html ships in git so `go build` works without
	// pnpm, so the tree existing does not prove the UI was built. Checking
	// for index.html at least distinguishes "no UI" from "broken embed".
	if _, err := fs.Stat(distFS, "index.html"); err != nil {
		return fmt.Errorf("dashboard index.html: %w", err)
	}
	dashboard.Register(e, distFS)
	return nil
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
	if s.cronSched != nil && s.cfg.Relays {
		go func() { s.cronDone <- s.cronSched.Start(ctx) }()
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
	if s.cronSched != nil {
		s.cronSched.Stop()
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
