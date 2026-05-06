// Admin DTOs — request/response shapes for /api/admin/* endpoints.
//
// Each DTO mirrors a schema in duragraph-spec/api/platform.yaml. The
// JSON tags must stay in sync with the spec; the engine emits these
// shapes verbatim and the dashboard's typed client deserialises them
// directly.
package admin

import "time"

// User mirrors the User schema in duragraph-spec/api/platform.yaml.
// TenantID is a pointer because pending users have no tenant — the
// spec models that as nullable, which JSON-omits to `null` (not a
// missing field).
type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	OAuthProvider string    `json:"oauth_provider"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	TenantID      *string   `json:"tenant_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// AdminUserListResponse mirrors AdminUserListResponse in the spec.
// Pagination fields (limit, offset) are echoed back so the client can
// confirm what window was applied (the server clamps spec-out-of-range
// values).
type AdminUserListResponse struct {
	Users  []User `json:"users"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// AdminActionRequest mirrors AdminActionRequest in the spec — used as
// the optional body for /reject and /suspend. `reason` is the only
// field; everything else is path/auth-derived.
type AdminActionRequest struct {
	Reason string `json:"reason"`
}

// TenantMetrics mirrors TenantMetrics in the spec.
type TenantMetrics struct {
	TenantID             string  `json:"tenant_id"`
	Window               string  `json:"window"`
	RunsPerSec           float64 `json:"runs_per_sec"`
	RunsActive           int64   `json:"runs_active"`
	AssistantsTotal      int64   `json:"assistants_total"`
	ThreadsTotal         int64   `json:"threads_total"`
	TokensConsumedPerSec float64 `json:"tokens_consumed_per_sec"`
}

// AdminMetricsResponse mirrors AdminMetricsResponse in the spec —
// per-tenant breakdown plus cross-tenant totals.
type AdminMetricsResponse struct {
	Window  string          `json:"window"`
	Tenants []TenantMetrics `json:"tenants"`
	Totals  MetricsTotals   `json:"totals"`
}

// MetricsTotals is the sum-across-tenants object in the spec — same
// shape as TenantMetrics minus tenant_id and window.
type MetricsTotals struct {
	RunsPerSec           float64 `json:"runs_per_sec"`
	RunsActive           int64   `json:"runs_active"`
	AssistantsTotal      int64   `json:"assistants_total"`
	ThreadsTotal         int64   `json:"threads_total"`
	TokensConsumedPerSec float64 `json:"tokens_consumed_per_sec"`
}

// ErrorResponse mirrors ErrorResponse in the spec. Local copy rather
// than reusing dto.ErrorResponse to keep the admin package free of
// LangGraph-DTO imports — the JSON shape on the wire is identical.
type ErrorResponse struct {
	Error   string         `json:"error"`
	Message string         `json:"message,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}
