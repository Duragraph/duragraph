package handlers

import (
	"net/http"
	"os"
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

// EngineMode describes how the engine is running.
//
// "multitenant" — MIGRATOR_PLATFORM_ENABLED=true, full platform features active.
// "serve"       — single-tenant `duragraph serve` (default for non-platform mode).
// "dev"         — `duragraph dev`. Phase 4 will populate this from the cobra
//
//	command; for now we never emit "dev" from /info.
type EngineMode string

const (
	ModeDev         EngineMode = "dev"
	ModeServe       EngineMode = "serve"
	ModeMultitenant EngineMode = "multitenant"
)

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
	// Mode reports the engine's runtime mode. See EngineMode.
	Mode EngineMode `json:"mode"`
	// PlatformEnabled is true when multi-tenant features (admin APIs, tenant
	// provisioning, etc.) are active. Driven by MIGRATOR_PLATFORM_ENABLED.
	PlatformEnabled bool `json:"platform_enabled"`
	// AuthEnabled mirrors the AUTH_ENABLED env toggle (JWT/auth gating).
	AuthEnabled bool `json:"auth_enabled"`
	// PasswordAuthEnabled mirrors the AUTH_PASSWORD_ENABLED env toggle —
	// whether /api/auth/register and /api/auth/login routes are registered.
	// The dashboard reads this to decide whether to render the password
	// login form vs. fall through to OAuth-only or no-auth UX.
	PasswordAuthEnabled bool `json:"password_auth_enabled"`
	// OAuthProviders lists configured OAuth providers in alphabetical order.
	// Always non-nil so the JSON shape is `[]` not `null`.
	OAuthProviders []string `json:"oauth_providers"`
}

// Ok handles GET /ok - simple health check
func (h *SystemHandler) Ok(c echo.Context) error {
	return c.JSON(http.StatusOK, OkResponse{Ok: true})
}

// Info handles GET /info - system information.
//
// Capability flags (mode, platform_enabled, auth_enabled, oauth_providers) are
// read from environment variables on every call. /info is low-frequency (the
// embedded dashboard fetches once at boot) so we avoid plumbing them through
// the constructor.
func (h *SystemHandler) Info(c echo.Context) error {
	platformEnabled := os.Getenv("MIGRATOR_PLATFORM_ENABLED") == "true"
	authEnabled := os.Getenv("AUTH_ENABLED") == "true"
	passwordAuthEnabled := os.Getenv("AUTH_PASSWORD_ENABLED") == "true"

	// alphabetical, stable for tests
	oauthProviders := []string{}
	if os.Getenv("OAUTH_GITHUB_CLIENT_ID") != "" {
		oauthProviders = append(oauthProviders, "github")
	}
	if os.Getenv("OAUTH_GOOGLE_CLIENT_ID") != "" {
		oauthProviders = append(oauthProviders, "google")
	}

	mode := ModeServe
	if platformEnabled {
		mode = ModeMultitenant
	}
	// TODO(phase-4): when `duragraph dev` is wired, pass a sentinel through the
	// cobra command so the handler can return ModeDev for that case.

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
		Mode:                mode,
		PlatformEnabled:     platformEnabled,
		AuthEnabled:         authEnabled,
		PasswordAuthEnabled: passwordAuthEnabled,
		OAuthProviders:      oauthProviders,
	})
}
