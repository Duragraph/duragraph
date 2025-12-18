package handlers

import (
	"net/http"
	"runtime"

	"github.com/labstack/echo/v4"
)

// SystemHandler handles system-related HTTP requests
type SystemHandler struct {
	version string
}

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler(version string) *SystemHandler {
	return &SystemHandler{
		version: version,
	}
}

// OkResponse represents the response for GET /ok
type OkResponse struct {
	Ok bool `json:"ok"`
}

// InfoResponse represents the response for GET /info
type InfoResponse struct {
	Version       string   `json:"version"`
	GoVersion     string   `json:"go_version"`
	Platform      string   `json:"platform"`
	Architecture  string   `json:"arch"`
	Capabilities  []string `json:"capabilities"`
	DefaultModel  string   `json:"default_model,omitempty"`
	RuntimeConfig struct {
		Checkpointer string `json:"checkpointer"`
		Store        string `json:"store"`
	} `json:"runtime_config"`
}

// Ok handles GET /ok - simple health check
func (h *SystemHandler) Ok(c echo.Context) error {
	return c.JSON(http.StatusOK, OkResponse{Ok: true})
}

// Info handles GET /info - system information
func (h *SystemHandler) Info(c echo.Context) error {
	return c.JSON(http.StatusOK, InfoResponse{
		Version:      h.version,
		GoVersion:    runtime.Version(),
		Platform:     runtime.GOOS,
		Architecture: runtime.GOARCH,
		Capabilities: []string{
			"assistants",
			"threads",
			"runs",
			"streaming",
			"human-in-the-loop",
		},
		RuntimeConfig: struct {
			Checkpointer string `json:"checkpointer"`
			Store        string `json:"store"`
		}{
			Checkpointer: "postgres",
			Store:        "postgres",
		},
	})
}
