package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"duragraph/runtime/bridge"
)

type Run struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id"`
	Assistant string    `json:"assistant_id"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Completed bool      `json:"completed"`
	LastError string    `json:"error,omitempty"`
}

var runStore = map[string]*Run{}

// Global bridge instance - now using the real runtime/bridge
var globalBridge *bridge.Bridge

func postRunHandler(c echo.Context) error {
	var body struct {
		ThreadID    string `json:"thread_id"`
		AssistantID string `json:"assistant_id"`
		Input       string `json:"input"`
	}

	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid json"})
	}

	id := uuid.New().String()
	run := &Run{
		ID:        id,
		ThreadID:  body.ThreadID,
		Assistant: body.AssistantID,
		Status:    "started",
		StartedAt: time.Now(),
	}
	runStore[id] = run

	// Execute workflow via bridge
	if globalBridge != nil {
		ctx := context.Background()
		req := bridge.WorkflowRequest{
			RunID:       id,
			ThreadID:    body.ThreadID,
			AssistantID: body.AssistantID,
			Input:       body.Input,
			Config:      make(map[string]interface{}),
		}

		result, err := globalBridge.ExecuteWorkflow(ctx, req)
		if err != nil {
			log.Printf("Failed to start workflow: %v", err)
			run.Status = "failed"
			run.LastError = err.Error()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start workflow"})
		}

		// Update run status from bridge result
		run.Status = result.Status
		log.Printf("Workflow started successfully: %s", result.RunID)
	}

	return c.JSON(http.StatusAccepted, map[string]string{"run_id": id})
}

func getRunHandler(c echo.Context) error {
	id := c.Param("id")
	run, ok := runStore[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	// If bridge is available, query real workflow status
	if globalBridge != nil {
		ctx := context.Background()
		result, err := globalBridge.QueryWorkflow(ctx, id)
		if err == nil {
			// Update run status from Temporal
			run.Status = result.Status
			if result.Error != "" {
				run.LastError = result.Error
			}
			if result.EndTime != nil {
				run.Completed = (result.Status == "completed" || result.Status == "failed")
			}
		}
	}

	return c.JSON(http.StatusOK, run)
}

func streamHandler(c echo.Context) error {
	runID := c.QueryParam("run_id")
	if runID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing run_id"})
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "stream unsupported"})
	}

	// Simulate three events
	fmt.Fprintf(w, "event: run_started\ndata: {\"run_id\":\"%s\"}\n\n", runID)
	flusher.Flush()
	time.Sleep(500 * time.Millisecond)

	fmt.Fprintf(w, "event: message_delta\ndata: {\"content\":\"echo\"}\n\n")
	flusher.Flush()
	time.Sleep(500 * time.Millisecond)

	fmt.Fprintf(w, "event: run_completed\ndata: {\"run_id\":\"%s\",\"status\":\"completed\"}\n\n", runID)
	flusher.Flush()

	// Update run status
	if run, ok := runStore[runID]; ok {
		run.Status = "completed"
		run.Completed = true
	}

	return nil
}

func healthHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status":  "healthy",
		"version": "0.1.0",
	})
}

func main() {
	// Initialize bridge with Temporal connection
	temporalHost := os.Getenv("TEMPORAL_HOSTPORT")
	if temporalHost == "" {
		temporalHost = "localhost:7233"
	}

	namespace := os.Getenv("TEMPORAL_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	var err error
	globalBridge, err = bridge.NewBridge(temporalHost, namespace)
	if err != nil {
		log.Printf("⚠️  Failed to initialize Temporal bridge: %v", err)
		log.Printf("🔄 Continuing without Temporal integration")
		globalBridge = nil
	} else {
		log.Printf("✅ Bridge connected to Temporal at %s", temporalHost)
	}

	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Routes
	e.GET("/health", healthHandler)
	e.POST("/runs", postRunHandler)
	e.GET("/runs/:id", getRunHandler)
	e.GET("/stream", streamHandler)

	// Graceful shutdown
	defer func() {
		if globalBridge != nil {
			globalBridge.Close()
			log.Printf("🔌 Bridge connection closed")
		}
	}()

	// Start server
	log.Printf("🚀 DuraGraph API Server starting on :8080")
	if globalBridge != nil {
		log.Printf("📊 Bridge initialized and ready for workflow execution")
	} else {
		log.Printf("📊 Running in standalone mode (no Temporal connection)")
	}
	e.Logger.Fatal(e.Start(":8080"))
}
