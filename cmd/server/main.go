package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/duragraph/duragraph/cmd/server/config"
	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/application/query"
	"github.com/duragraph/duragraph/internal/application/service"
	"github.com/duragraph/duragraph/internal/infrastructure/auth"
	infra_exec "github.com/duragraph/duragraph/internal/infrastructure/execution"
	"github.com/duragraph/duragraph/internal/infrastructure/graph"
	"github.com/duragraph/duragraph/internal/infrastructure/http/handlers"
	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	"github.com/duragraph/duragraph/internal/infrastructure/mcp"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	"github.com/duragraph/duragraph/internal/infrastructure/monitoring"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	"github.com/duragraph/duragraph/internal/infrastructure/streaming"
	"github.com/duragraph/duragraph/internal/infrastructure/tools"
	"github.com/duragraph/duragraph/internal/infrastructure/tracing"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	fmt.Println("🚀 DuraGraph Server - DDD Architecture")
	fmt.Printf("📍 Server: %s\n", cfg.ServerAddr())
	fmt.Printf("🗄️  Database: %s:%d/%s\n", cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)
	fmt.Printf("📨 NATS: %s\n", cfg.NATS.URL)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	// Initialize OpenTelemetry tracing (opt-in via OTEL_ENABLED)
	if os.Getenv("OTEL_ENABLED") == "true" {
		shutdownTracer, err := tracing.Init(ctx, "duragraph-server", version)
		if err != nil {
			log.Printf("failed to initialize tracing: %v", err)
		} else {
			defer shutdownTracer(context.Background())
			fmt.Println("✅ OpenTelemetry tracing enabled")
		}
	}

	// Initialize PostgreSQL connection pools (write + read)
	writeConfig := postgres.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		Database: cfg.Database.Database,
		SSLMode:  cfg.Database.SSLMode,
	}

	var readConfig *postgres.Config
	if cfg.ReadDatabase != nil {
		readConfig = &postgres.Config{
			Host:     cfg.ReadDatabase.Host,
			Port:     cfg.ReadDatabase.Port,
			User:     cfg.ReadDatabase.User,
			Password: cfg.ReadDatabase.Password,
			Database: cfg.ReadDatabase.Database,
			SSLMode:  cfg.ReadDatabase.SSLMode,
		}
	}

	// Run schema migrations BEFORE opening the application pool.
	//
	// The migrator owns DB-level provisioning + golang-migrate-driven
	// schema rollout, replacing the docker-entrypoint-initdb.d mount
	// that previously seeded the SQL on first container boot. See
	// internal/infrastructure/persistence/postgres/migrator.go.
	//
	// Behaviour:
	//   - Always runs MigrateMainDB(ctx, DB_NAME) — drop-in replacement
	//     for the old initdb seed; existing single-DB deployments
	//     continue to work without changes.
	//   - When MIGRATOR_PLATFORM_ENABLED=true, additionally runs
	//     Bootstrap (creates duragraph_platform if absent + applies
	//     platform migrations, today a no-op) and MigrateAllTenants
	//     (per-tenant migrations for approved tenants from
	//     platform.tenants). Default false until feat/platform-db-init
	//     and downstream multi-tenant routing land.
	platformEnabled := os.Getenv("MIGRATOR_PLATFORM_ENABLED") == "true"
	adminDSN := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/postgres?sslmode=%s",
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Host, cfg.Database.Port,
		cfg.Database.SSLMode,
	)
	migrator, err := postgres.NewMigrator(adminDSN)
	if err != nil {
		log.Fatalf("failed to construct migrator: %v", err)
	}

	if platformEnabled {
		if err := migrator.Bootstrap(ctx); err != nil {
			log.Fatalf("migrator bootstrap failed: %v", err)
		}
		fmt.Println("✅ Platform DB bootstrapped")
	}

	if err := migrator.MigrateMainDB(ctx, cfg.Database.Database); err != nil {
		log.Fatalf("main DB migrations failed: %v", err)
	}
	fmt.Printf("✅ Main DB migrations applied (%s)\n", cfg.Database.Database)

	if platformEnabled {
		results := migrator.MigrateAllTenants(ctx)
		failed := 0
		for _, r := range results {
			if r.Err != nil {
				failed++
			}
		}
		fmt.Printf("✅ Tenant migrations dispatched (%d total, %d failed)\n", len(results), failed)
	}

	pools, err := postgres.NewPools(ctx, writeConfig, readConfig)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer postgres.ClosePools(pools)

	if readConfig != nil {
		fmt.Println("✅ Database connected (write + read replica)")
	} else {
		fmt.Println("✅ Database connected (single instance)")
	}

	// Initialize event bus
	eventBus := eventbus.New()

	// Initialize event store and outbox (always use write pool)
	eventStore := postgres.NewEventStore(pools.Write)
	outbox := postgres.NewOutbox(pools.Write)

	// Initialize repositories with read/write split
	runRepo := postgres.NewRunRepositoryWithPools(pools.Write, pools.Read, eventStore)
	assistantRepo := postgres.NewAssistantRepositoryWithPools(pools.Write, pools.Read, eventStore)
	threadRepo := postgres.NewThreadRepositoryWithPools(pools.Write, pools.Read, eventStore)
	graphRepo := postgres.NewGraphRepositoryWithPools(pools.Write, pools.Read, eventStore)
	interruptRepo := postgres.NewInterruptRepositoryWithPools(pools.Write, pools.Read, eventStore)
	checkpointRepo := postgres.NewCheckpointRepositoryWithPools(pools.Write, pools.Read)

	// Initialize persistent worker + task repositories
	workerRepo := postgres.NewWorkerRepository(pools.Write)
	taskRepo := postgres.NewTaskAssignmentRepository(pools.Write)

	// Initialize NATS publisher (Watermill)
	logger := watermill.NewStdLogger(false, false)
	publisher, err := nats.NewPublisher(cfg.NATS.URL, logger)
	if err != nil {
		log.Fatalf("failed to create NATS publisher: %v", err)
	}
	defer publisher.Close()

	fmt.Println("✅ NATS publisher connected")

	// Initialize NATS subscriber (Watermill)
	subscriber, err := nats.NewSubscriber(cfg.NATS.URL, "duragraph-server", logger)
	if err != nil {
		log.Fatalf("failed to create NATS subscriber: %v", err)
	}
	defer subscriber.Close()

	fmt.Println("✅ NATS subscriber connected")

	// Initialize NATS task queue (raw nats.go for JetStream work queue)
	taskQueue, err := nats.NewTaskQueue(cfg.NATS.URL)
	if err != nil {
		log.Fatalf("failed to create NATS task queue: %v", err)
	}
	defer taskQueue.Close()

	fmt.Println("✅ NATS task queue connected")

	// Start outbox relay worker
	outboxRelay := messaging.NewOutboxRelay(outbox, publisher, 1*time.Second, 10)
	go func() {
		if err := outboxRelay.Start(ctx); err != nil {
			log.Printf("outbox relay error: %v", err)
		}
	}()

	fmt.Println("✅ Outbox relay worker started")

	// Platform-mode wiring: connect a separate pool to the platform DB
	// (`duragraph_platform`) so the User/Tenant repositories can drive
	// platform-admin commands and so the tenant-provisioner subscriber
	// can complete the async provisioning workflow kicked off by an
	// admin's Approve click.
	//
	// Gated behind MIGRATOR_PLATFORM_ENABLED — same flag the migrator
	// uses for Bootstrap and MigrateAllTenants. The platform pool, the
	// User/Tenant repositories and the TenantProvisioner are all
	// optional today; existing single-DB deployments continue to work
	// unchanged when the flag is off.
	//
	// When the flag is on, this block establishes:
	//   - `platformPool` — pgxpool against the duragraph_platform DB
	//   - userRepo + tenantRepo — projection writers for platform.users
	//     and platform.tenants
	//   - tenantProvisioner — NATS subscriber that turns
	//     tenant.provisioning events into CREATE DATABASE + migrate +
	//     (stubbed) NATS Account creation, then approves the tenant.
	//
	// The HTTP admin handlers (POST /api/admin/users/.../approve etc.)
	// are NOT wired here — that's the next PR (feat/admin-handlers).
	// The repos and the publisher are constructed so the handlers can
	// pick them up; the publisher reused for command-side publishing
	// is the existing `publisher` constructed above.
	if platformEnabled {
		platformConfig := postgres.Config{
			Host:     cfg.Database.Host,
			Port:     cfg.Database.Port,
			User:     cfg.Database.User,
			Password: cfg.Database.Password,
			Database: "duragraph_platform",
			SSLMode:  cfg.Database.SSLMode,
		}
		platformPool, err := postgres.NewPool(ctx, platformConfig)
		if err != nil {
			log.Fatalf("failed to connect to platform DB: %v", err)
		}
		defer platformPool.Close()
		fmt.Println("✅ Platform DB connected (duragraph_platform)")

		userRepo := postgres.NewUserRepository(platformPool)
		tenantRepo := postgres.NewTenantRepository(platformPool)

		// Suppress unused-variable warnings until feat/admin-handlers
		// wires the HTTP handlers that consume these.
		_ = userRepo
		_ = tenantRepo

		// Tenant provisioner subscriber. Uses a JetStream durable
		// consumer (durable name "tenant-provisioner") bound to the
		// existing duragraph-events stream — so tenant.provisioning
		// events published while the engine was offline still get
		// delivered when it comes back. Separate from the plain-NATS
		// `subscriber` used by run/execution events because the
		// platform-admin loop needs at-least-once durability.
		jsSubscriber, err := nats.NewJetStreamSubscriber(nats.JetStreamSubscriberConfig{
			URL:           cfg.NATS.URL,
			StreamName:    messaging.JetStreamStreamName,
			FilterSubject: messaging.TenantProvisioningTopic,
			Durable:       messaging.DurableName,
			MaxDeliver:    10, // bound poison-message redelivery loops
		})
		if err != nil {
			log.Fatalf("failed to construct tenant provisioner JetStream subscriber: %v", err)
		}
		defer jsSubscriber.Close()

		tenantProvisioner := messaging.NewTenantProvisioner(
			jsSubscriber,
			tenantRepo,
			migrator,
			messaging.NoopNATSAccountProvisioner{}, // operator-JWT wiring is a follow-up PR
			log.Default(),
		)
		go func() {
			if err := tenantProvisioner.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("tenant provisioner error: %v", err)
			}
		}()
		fmt.Println("✅ Tenant provisioner JetStream subscriber started (durable: tenant-provisioner)")
	}

	// Start cleanup worker
	cleanupWorker := messaging.NewCleanupWorker(outbox, 1*time.Hour, 7)
	go func() {
		if err := cleanupWorker.Start(ctx); err != nil {
			log.Printf("cleanup worker error: %v", err)
		}
	}()

	fmt.Println("✅ Cleanup worker started")

	// Initialize streaming bridge (connects eventBus to NATS for real-time streaming)
	streamingBridge := streaming.NewStreamingBridge(eventBus, publisher)
	streamingBridge.Start()

	fmt.Println("✅ Streaming bridge started")

	// Initialize worker service (persistent PostgreSQL + NATS)
	healthThreshold := 90 * time.Second
	if v := os.Getenv("WORKER_HEALTH_THRESHOLD_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			healthThreshold = time.Duration(secs) * time.Second
		}
	}
	workerService := service.NewWorkerService(
		workerRepo,
		taskRepo,
		runRepo,
		assistantRepo,
		taskQueue,
		healthThreshold,
	)

	fmt.Println("✅ Worker service initialized (PostgreSQL + NATS)")

	// Start lease monitor goroutine (uses advisory lock for single-instance execution)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var acquired bool
				if err := pools.Write.QueryRow(ctx, `SELECT pg_try_advisory_lock(42)`).Scan(&acquired); err != nil || !acquired {
					continue
				}
				if monErr := workerService.MonitorExpiredLeases(ctx); monErr != nil {
					log.Printf("lease monitor error: %v", monErr)
				}
				pools.Write.Exec(ctx, `SELECT pg_advisory_unlock(42)`)
			}
		}
	}()

	fmt.Println("✅ Lease monitor started (30s interval)")

	// Start stale worker cleanup goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed, err := workerRepo.CleanupStale(ctx, 5*time.Minute)
				if err != nil {
					log.Printf("stale worker cleanup error: %v", err)
				} else if removed > 0 {
					log.Printf("cleaned up %d stale workers", removed)
				}
			}
		}
	}()

	// Initialize Prometheus metrics
	metrics := monitoring.NewMetrics("duragraph")

	// Register DB pool collectors for Prometheus
	writePoolCollector := monitoring.NewDBPoolCollector(pools.Write, "duragraph", "write")
	prometheus.MustRegister(writePoolCollector)
	if pools.Read != pools.Write {
		readPoolCollector := monitoring.NewDBPoolCollector(pools.Read, "duragraph", "read")
		prometheus.MustRegister(readPoolCollector)
	}

	fmt.Println("✅ Prometheus metrics + DB pool collectors registered")

	// Initialize tool registry with built-in tools
	toolRegistry := tools.NewRegistry()
	if err := tools.RegisterBuiltinTools(toolRegistry); err != nil {
		log.Fatalf("failed to register built-in tools: %v", err)
	}

	fmt.Println("✅ Tool registry initialized")

	// Initialize LLM executor
	llmExecutor := infra_exec.NewLLMExecutor(
		os.Getenv("OPENAI_API_KEY"),
		os.Getenv("ANTHROPIC_API_KEY"),
	)

	// Initialize tool executor
	toolExecutor := infra_exec.NewToolExecutor(toolRegistry)

	// Initialize graph execution engine
	graphEngine := graph.NewEngine(eventBus)

	// Initialize command handlers
	createRunHandler := command.NewCreateRunHandler(runRepo)
	submitToolOutputsHandler := command.NewSubmitToolOutputsHandler(runRepo, interruptRepo)
	deleteRunHandler := command.NewDeleteRunHandler(runRepo)
	createAssistantHandler := command.NewCreateAssistantHandler(assistantRepo)
	updateAssistantHandler := command.NewUpdateAssistantHandler(assistantRepo)
	deleteAssistantHandler := command.NewDeleteAssistantHandler(assistantRepo)
	createThreadHandler := command.NewCreateThreadHandler(threadRepo)
	updateThreadHandler := command.NewUpdateThreadHandler(threadRepo)
	addMessageHandler := command.NewAddMessageHandler(threadRepo)

	// Initialize query handlers
	getRunHandler := query.NewGetRunHandler(runRepo)
	listRunsHandler := query.NewListRunsHandler(runRepo)
	getAssistantHandler := query.NewGetAssistantHandler(assistantRepo)
	listAssistantsHandler := query.NewListAssistantsHandler(assistantRepo)
	searchAssistantsHandler := query.NewSearchAssistantsHandler(assistantRepo)
	countAssistantsHandler := query.NewCountAssistantsHandler(assistantRepo)
	getThreadHandler := query.NewGetThreadHandler(threadRepo)
	listThreadsHandler := query.NewListThreadsHandler(threadRepo)
	searchThreadsHandler := query.NewSearchThreadsHandler(threadRepo)
	countThreadsHandler := query.NewCountThreadsHandler(threadRepo)
	deleteThreadHandler := command.NewDeleteThreadHandler(threadRepo)

	// Initialize checkpoint handlers
	getThreadStateHandler := query.NewGetThreadStateHandler(checkpointRepo)
	getThreadHistoryHandler := query.NewGetThreadHistoryHandler(checkpointRepo)
	updateThreadStateHandler := command.NewUpdateThreadStateHandler(checkpointRepo)
	createCheckpointHandler := command.NewCreateCheckpointHandler(checkpointRepo)
	copyThreadHandler := command.NewCopyThreadHandler(threadRepo, checkpointRepo)

	// Initialize assistant versioning handlers
	createAssistantVersionHandler := command.NewCreateAssistantVersionHandler(assistantRepo)
	setLatestVersionHandler := command.NewSetLatestVersionHandler(assistantRepo)
	getAssistantVersionsHandler := query.NewGetAssistantVersionsHandler(assistantRepo)
	getAssistantSchemaHandler := query.NewGetAssistantSchemaHandler(assistantRepo, graphRepo)

	// Initialize graph query handlers
	getAssistantGraphHandler := query.NewGetAssistantGraphHandler(assistantRepo, graphRepo)
	getSubgraphsHandler := query.NewGetSubgraphsHandler(assistantRepo, graphRepo)

	// Initialize application services
	runService := service.NewRunService(
		runRepo,
		graphRepo,
		assistantRepo,
		interruptRepo,
		graphEngine,
		eventBus,
	)

	// Wire up worker service and task queue for remote execution + NATS-based WaitForRun
	runService.SetWorkerService(workerService)
	runService.SetTaskQueue(taskQueue)

	fmt.Println("✅ Run service configured with worker dispatch + NATS events")

	// Initialize store repository
	storeRepo := postgres.NewStoreRepositoryWithPools(pools.Write, pools.Read)

	// Initialize cron repository and scheduler
	cronRepo := postgres.NewCronRepositoryWithPools(pools.Write, pools.Read)
	cronScheduler := service.NewCronScheduler(cronRepo, 30*time.Second)
	go func() {
		if err := cronScheduler.Start(ctx); err != nil {
			log.Printf("cron scheduler error: %v", err)
		}
	}()

	fmt.Println("✅ Cron scheduler started (30s poll interval)")

	// Initialize HTTP handlers
	runHandler := handlers.NewRunHandler(
		createRunHandler,
		createThreadHandler,
		submitToolOutputsHandler,
		deleteRunHandler,
		getRunHandler,
		listRunsHandler,
		runService,
		subscriber,
		eventBus,
	)
	assistantHandler := handlers.NewAssistantHandler(
		createAssistantHandler,
		updateAssistantHandler,
		deleteAssistantHandler,
		createAssistantVersionHandler,
		setLatestVersionHandler,
		getAssistantHandler,
		listAssistantsHandler,
		searchAssistantsHandler,
		countAssistantsHandler,
		getAssistantVersionsHandler,
		getAssistantSchemaHandler,
		getAssistantGraphHandler,
		getSubgraphsHandler,
	)
	threadHandler := handlers.NewThreadHandler(
		createThreadHandler,
		updateThreadHandler,
		deleteThreadHandler,
		addMessageHandler,
		getThreadHandler,
		listThreadsHandler,
		searchThreadsHandler,
		countThreadsHandler,
	)
	streamHandler := handlers.NewStreamHandler(subscriber)
	systemHandler := handlers.NewSystemHandler("2.0.0-ddd")
	threadStateHandler := handlers.NewThreadStateHandler(
		getThreadStateHandler,
		getThreadHistoryHandler,
		updateThreadStateHandler,
		createCheckpointHandler,
		copyThreadHandler,
	)
	workerHandler := handlers.NewWorkerHandler(
		workerService,
		30*time.Second, // Health threshold
		fmt.Sprintf("http://%s", cfg.ServerAddr()),
	)

	// Initialize Echo server
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = middleware.ErrorHandler()

	// Middleware
	e.Use(middleware.RequestID())
	e.Use(middleware.SecurityHeaders())
	e.Use(middleware.RequestValidation(10 * 1024 * 1024)) // 10 MB
	e.Use(middleware.Logger())
	e.Use(middleware.Metrics(metrics))
	e.Use(echomiddleware.Recover())
	e.Use(echomiddleware.CORSWithConfig(echomiddleware.CORSConfig{
		AllowOrigins: func() []string {
			if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
				return strings.Split(origins, ",")
			}
			return []string{"*"}
		}(),
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type", "X-API-Key", "X-Request-ID"},
		MaxAge:       3600,
	}))

	if os.Getenv("OTEL_ENABLED") == "true" {
		e.Use(otelecho.Middleware("duragraph-server"))
	}

	// Optional rate limiting (configurable via env vars)
	if os.Getenv("RATE_LIMIT_ENABLED") == "true" {
		rps := 10.0
		burst := 20
		if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
			if parsed, err := strconv.ParseFloat(v, 64); err == nil {
				rps = parsed
			}
		}
		if v := os.Getenv("RATE_LIMIT_BURST"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil {
				burst = parsed
			}
		}
		e.Use(middleware.SimpleRateLimit(rps, burst))
		fmt.Printf("✅ Rate limiting enabled (%.0f req/s, burst %d)\n", rps, burst)
	}

	// Authentication.
	//
	// When AUTH_ENABLED=true, every authenticated request flows through
	// TenantMiddleware: it verifies the platform JWT (HS256, iss claim
	// must equal "duragraph-platform" — see auth/jwt.yml) and populates
	// the request context with user_id, tenant_id, role, and email.
	//
	// We deliberately replace the legacy middleware.OptionalAuth here.
	// OptionalAuth uses the older middleware.JWTClaims shape ({user_id,
	// username, email, roles}); the new platform contract uses
	// auth.Claims ({user_id, tenant_id, role, email}). Stacking both
	// would attempt to verify each request twice with mutually-
	// incompatible structs and reject every new-shape token. The legacy
	// JWT/OptionalAuth/APIKeyAuth helpers remain in middleware/auth.go
	// for now (their unit tests still cover them), but they are no
	// longer wired into main.go's AUTH_ENABLED branch.
	//
	// Public/auth-only routes (/health, /metrics, /api/auth/*) bypass
	// TenantMiddleware via Echo's per-route middleware semantics: they
	// are registered on the bare *echo.Echo (not under a group with
	// TenantMiddleware applied). Future /api/auth/login, /callback,
	// /logout endpoints will live alongside /health below.
	//
	// Backwards compat: when AUTH_ENABLED=false (the default), no auth
	// middleware runs at all. Existing single-tenant deployments keep
	// working unchanged. This gate is intentionally NOT tied to
	// MIGRATOR_PLATFORM_ENABLED — middleware applies whether or not
	// the multi-tenant migrator is active. The two flags are
	// orthogonal: AUTH_ENABLED gates JWT verification, the migrator
	// flag gates platform-DB provisioning.
	authEnabled := os.Getenv("AUTH_ENABLED") == "true"
	var verifier *auth.Verifier
	if authEnabled {
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = "default-secret-change-in-production"
		}
		v, err := auth.NewVerifier([]byte(jwtSecret))
		if err != nil {
			log.Fatalf("failed to construct JWT verifier: %v", err)
		}
		verifier = v
		fmt.Println("✅ Authentication enabled (TenantMiddleware)")
	}

	// Routes
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status":  "healthy",
			"version": "2.0.0-ddd",
		})
	})

	// Prometheus metrics endpoint
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// System endpoints (LangGraph compatible)
	e.GET("/ok", systemHandler.Ok)
	e.GET("/info", systemHandler.Info)

	// API routes.
	//
	// Build the /api/v1 group with platform middleware when AUTH_ENABLED.
	// Order matters: TenantMiddleware MUST run before RequireTenant
	// (RequireTenant reads what TenantMiddleware writes). RequireTenant
	// is /api/v1-only — pending users still need /api/platform/me etc.
	var apiMiddleware []echo.MiddlewareFunc
	if authEnabled {
		apiMiddleware = append(apiMiddleware, middleware.TenantMiddleware(verifier))
		apiMiddleware = append(apiMiddleware, middleware.RequireTenant())
	}
	api := e.Group("/api/v1", apiMiddleware...)

	// Thread Run routes (LangGraph compatible)
	api.POST("/threads/:thread_id/runs", runHandler.CreateRun)
	api.GET("/threads/:thread_id/runs", runHandler.ListRuns)
	api.GET("/threads/:thread_id/runs/:run_id", runHandler.GetRun)
	api.POST("/threads/:thread_id/runs/:run_id/cancel", runHandler.CancelRun)
	api.POST("/threads/:thread_id/runs/:run_id/resume", runHandler.ResumeRun)
	api.GET("/threads/:thread_id/runs/:run_id/join", runHandler.JoinRun)
	api.DELETE("/threads/:thread_id/runs/:run_id", runHandler.DeleteRun)

	// Stateless Run routes (LangGraph compatible)
	api.GET("/runs", runHandler.ListAllRuns)
	api.GET("/runs/:run_id", runHandler.GetRun)
	api.POST("/runs", runHandler.CreateStatelessRun)
	api.POST("/runs/wait", runHandler.CreateRunAndWait)
	api.POST("/runs/stream", runHandler.CreateStatelessRunWithStream)
	api.POST("/runs/batch", runHandler.CreateBatchRuns)
	api.POST("/runs/cancel", runHandler.CancelStatelessRuns)

	// Stream routes (LangGraph compatible)
	api.POST("/threads/:thread_id/runs/stream", runHandler.CreateRunWithStream)
	api.GET("/threads/:thread_id/runs/:run_id/stream", streamHandler.StreamRun)
	api.GET("/threads/:thread_id/stream", streamHandler.JoinThreadStream)
	api.GET("/stream", streamHandler.Stream) // Legacy SSE endpoint

	// Thread run wait route (LangGraph compatible)
	api.POST("/threads/:thread_id/runs/wait", runHandler.CreateThreadRunAndWait)

	// Assistant routes
	api.POST("/assistants", assistantHandler.Create)
	api.POST("/assistants/search", assistantHandler.Search)
	api.POST("/assistants/count", assistantHandler.Count)
	api.GET("/assistants/:assistant_id", assistantHandler.Get)
	api.GET("/assistants", assistantHandler.List)
	api.PATCH("/assistants/:assistant_id", assistantHandler.Update)
	api.DELETE("/assistants/:assistant_id", assistantHandler.Delete)

	// Assistant versioning routes (LangGraph compatible)
	api.POST("/assistants/:assistant_id/versions", assistantHandler.CreateVersion)
	api.GET("/assistants/:assistant_id/versions", assistantHandler.GetVersions)
	api.POST("/assistants/:assistant_id/latest", assistantHandler.SetLatestVersion)
	api.GET("/assistants/:assistant_id/schemas", assistantHandler.GetSchemas)

	// Assistant graph routes (LangGraph compatible)
	api.GET("/assistants/:assistant_id/graph", assistantHandler.GetGraph)
	api.GET("/assistants/:assistant_id/subgraphs", assistantHandler.GetSubgraphs)
	api.GET("/assistants/:assistant_id/subgraphs/:namespace", assistantHandler.GetSubgraph)

	// Thread routes
	api.POST("/threads", threadHandler.Create)
	api.POST("/threads/search", threadHandler.Search)
	api.POST("/threads/count", threadHandler.Count)
	api.GET("/threads/:thread_id", threadHandler.Get)
	api.GET("/threads", threadHandler.List)
	api.PATCH("/threads/:thread_id", threadHandler.Update)
	api.DELETE("/threads/:thread_id", threadHandler.Delete)
	api.POST("/threads/:thread_id/messages", threadHandler.AddMessage)

	// Thread state routes (LangGraph compatible)
	api.GET("/threads/:thread_id/state", threadStateHandler.GetState)
	api.POST("/threads/:thread_id/state", threadStateHandler.UpdateState)
	api.GET("/threads/:thread_id/state/:checkpoint_id", threadStateHandler.GetStateAtCheckpoint)
	api.POST("/threads/:thread_id/state/checkpoint", threadStateHandler.CreateCheckpoint)
	api.GET("/threads/:thread_id/history", threadStateHandler.GetHistory)
	api.POST("/threads/:thread_id/history", threadStateHandler.PostHistory)
	api.POST("/threads/:thread_id/copy", threadStateHandler.CopyThread)

	// Cron routes (LangGraph compatible)
	cronHandler := handlers.NewCronHandler(cronRepo)
	api.POST("/runs/crons", cronHandler.CreateStatelessCron)
	api.POST("/runs/crons/search", cronHandler.SearchCrons)
	api.POST("/runs/crons/count", cronHandler.CountCrons)
	api.DELETE("/runs/crons/:cron_id", cronHandler.DeleteCron)
	api.PATCH("/runs/crons/:cron_id", cronHandler.UpdateCron)
	api.POST("/threads/:thread_id/runs/crons", cronHandler.CreateThreadCron)

	// Store routes (LangGraph compatible)
	storeHandler := handlers.NewStoreHandler(storeRepo)
	api.PUT("/store/items", storeHandler.PutItem)
	api.GET("/store/items", storeHandler.GetItem)
	api.DELETE("/store/items", storeHandler.DeleteItem)
	api.POST("/store/items/search", storeHandler.SearchItems)
	api.POST("/store/namespaces", storeHandler.ListNamespaces)

	// MCP routes (Model Context Protocol - Streamable HTTP transport)
	mcpServer := mcp.NewServer(
		toolRegistry,
		getAssistantHandler,
		listAssistantsHandler,
		getThreadHandler,
		createRunHandler,
		getRunHandler,
		runService,
	)
	mcpHandler := handlers.NewMCPHandler(mcpServer)
	e.POST("/mcp", mcpHandler.Post)
	e.GET("/mcp", mcpHandler.Get)
	e.DELETE("/mcp", mcpHandler.Delete)

	fmt.Println("✅ MCP endpoint enabled at /mcp")

	// Worker protocol routes
	api.POST("/workers/register", workerHandler.Register)
	api.GET("/workers", workerHandler.ListWorkers)
	api.GET("/workers/:worker_id", workerHandler.GetWorker)
	api.POST("/workers/:worker_id/heartbeat", workerHandler.Heartbeat)
	api.POST("/workers/:worker_id/poll", workerHandler.Poll)
	api.POST("/workers/:worker_id/deregister", workerHandler.Deregister)
	api.POST("/workers/:worker_id/events", workerHandler.ReceiveEvent)
	api.GET("/workers/graphs/:graph_id", workerHandler.GetGraphDefinition)

	// Admin route group.
	//
	// Empty today — handlers land in the next PR alongside the platform
	// User/Tenant repositories. The middleware is wired now so the chain
	// is ready: TenantMiddleware verifies the JWT, then
	// AdminAuthMiddleware enforces role=admin. Until routes are added,
	// any request hitting /api/admin/* returns 404 (Echo's default for
	// no-match) which is fine.
	if authEnabled {
		_ = e.Group("/api/admin",
			middleware.TenantMiddleware(verifier),
			middleware.AdminAuthMiddleware(),
		)
	}

	// Start server
	go func() {
		fmt.Printf("🌐 Server listening on %s\n", cfg.ServerAddr())
		if err := e.Start(cfg.ServerAddr()); err != nil {
			log.Printf("server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 Shutting down gracefully...")

	// Cancel context to stop background goroutines
	ctxCancel()

	// Shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	// Stop workers
	outboxRelay.Stop()
	cleanupWorker.Stop()
	cronScheduler.Stop()

	fmt.Println("👋 Shutdown complete")

	// Suppress unused variable warnings for optional components
	_ = llmExecutor
	_ = toolExecutor
	_ = streamingBridge
}
