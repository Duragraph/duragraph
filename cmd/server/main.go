package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/duragraph/duragraph/cmd/server/config"
	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/application/query"
	"github.com/duragraph/duragraph/internal/application/service"
	infra_exec "github.com/duragraph/duragraph/internal/infrastructure/execution"
	"github.com/duragraph/duragraph/internal/infrastructure/graph"
	"github.com/duragraph/duragraph/internal/infrastructure/http/handlers"
	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	"github.com/duragraph/duragraph/internal/infrastructure/monitoring"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	"github.com/duragraph/duragraph/internal/infrastructure/tools"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	ctx := context.Background()

	// Initialize PostgreSQL connection pool
	pool, err := postgres.NewPool(ctx, postgres.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		Database: cfg.Database.Database,
		SSLMode:  cfg.Database.SSLMode,
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer postgres.Close(pool)

	fmt.Println("✅ Database connected")

	// Initialize event bus
	eventBus := eventbus.New()

	// Initialize event store and outbox
	eventStore := postgres.NewEventStore(pool)
	outbox := postgres.NewOutbox(pool)

	// Initialize repositories
	runRepo := postgres.NewRunRepository(pool, eventStore)
	assistantRepo := postgres.NewAssistantRepository(pool, eventStore)
	threadRepo := postgres.NewThreadRepository(pool, eventStore)
	graphRepo := postgres.NewGraphRepository(pool, eventStore)
	interruptRepo := postgres.NewInterruptRepository(pool, eventStore)
	checkpointRepo := postgres.NewCheckpointRepository(pool)

	// Initialize NATS publisher
	logger := watermill.NewStdLogger(false, false)
	publisher, err := nats.NewPublisher(cfg.NATS.URL, logger)
	if err != nil {
		log.Fatalf("failed to create NATS publisher: %v", err)
	}
	defer publisher.Close()

	fmt.Println("✅ NATS publisher connected")

	// Initialize NATS subscriber
	subscriber, err := nats.NewSubscriber(cfg.NATS.URL, "duragraph-server", logger)
	if err != nil {
		log.Fatalf("failed to create NATS subscriber: %v", err)
	}
	defer subscriber.Close()

	fmt.Println("✅ NATS subscriber connected")

	// Start outbox relay worker
	outboxRelay := messaging.NewOutboxRelay(outbox, publisher, 1*time.Second, 10)
	go func() {
		if err := outboxRelay.Start(ctx); err != nil {
			log.Printf("outbox relay error: %v", err)
		}
	}()

	fmt.Println("✅ Outbox relay worker started")

	// Start cleanup worker
	cleanupWorker := messaging.NewCleanupWorker(outbox, 1*time.Hour, 7)
	go func() {
		if err := cleanupWorker.Start(ctx); err != nil {
			log.Printf("cleanup worker error: %v", err)
		}
	}()

	fmt.Println("✅ Cleanup worker started")

	// Initialize Prometheus metrics
	metrics := monitoring.NewMetrics("duragraph")

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

	// Initialize application services
	runService := service.NewRunService(
		runRepo,
		graphRepo,
		assistantRepo,
		interruptRepo,
		graphEngine,
		eventBus,
	)

	// Initialize HTTP handlers
	runHandler := handlers.NewRunHandler(
		createRunHandler,
		createThreadHandler,
		submitToolOutputsHandler,
		deleteRunHandler,
		getRunHandler,
		listRunsHandler,
		runService,
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

	// Initialize Echo server
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = middleware.ErrorHandler()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Metrics(metrics))
	e.Use(echomiddleware.Recover())
	e.Use(echomiddleware.CORS())

	// Optional authentication (can be made required by setting env var)
	authEnabled := os.Getenv("AUTH_ENABLED") == "true"
	if authEnabled {
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = "default-secret-change-in-production"
		}
		e.Use(middleware.OptionalAuth(jwtSecret))
		fmt.Println("✅ Authentication enabled")
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

	// API routes
	api := e.Group("/api/v1")

	// Thread Run routes (LangGraph compatible)
	api.POST("/threads/:thread_id/runs", runHandler.CreateRun)
	api.GET("/threads/:thread_id/runs", runHandler.ListRuns)
	api.GET("/threads/:thread_id/runs/:run_id", runHandler.GetRun)
	api.POST("/threads/:thread_id/runs/:run_id/cancel", runHandler.CancelRun)
	api.GET("/threads/:thread_id/runs/:run_id/join", runHandler.JoinRun)
	api.DELETE("/threads/:thread_id/runs/:run_id", runHandler.DeleteRun)

	// Stateless Run routes (LangGraph compatible)
	api.POST("/runs", runHandler.CreateStatelessRun)
	api.POST("/runs/wait", runHandler.CreateRunAndWait)
	api.POST("/runs/stream", runHandler.CreateStatelessRunWithStream)
	api.POST("/runs/batch", runHandler.CreateBatchRuns)
	api.POST("/runs/cancel", runHandler.CancelStatelessRuns)

	// Stream routes (LangGraph compatible)
	api.POST("/threads/:thread_id/runs/stream", runHandler.CreateRunWithStream)
	api.GET("/threads/:thread_id/runs/:run_id/stream", streamHandler.StreamRun)
	api.GET("/stream", streamHandler.Stream) // Legacy SSE endpoint

	// Human-in-the-loop (state update)
	api.POST("/threads/:thread_id/state", runHandler.UpdateState)

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

	// Shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	// Stop workers
	outboxRelay.Stop()
	cleanupWorker.Stop()

	fmt.Println("👋 Shutdown complete")

	// Suppress unused variable warnings for optional components
	_ = llmExecutor
	_ = toolExecutor
}
