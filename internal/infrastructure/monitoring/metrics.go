package monitoring

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	RunsTotal            *prometheus.CounterVec
	RunDuration          *prometheus.HistogramVec
	RunsActive           *prometheus.GaugeVec
	RunStatusTransitions *prometheus.CounterVec

	// AssistantsTotal / ThreadsTotal are per-tenant gauges driven by the
	// command handlers (CreateAssistant/Delete, CreateThread/Delete).
	// The admin metrics endpoint queries these via Mimir as
	// `sum by (tenant_id) (duragraph_{assistants,threads}_total)`.
	AssistantsTotal *prometheus.GaugeVec
	ThreadsTotal    *prometheus.GaugeVec

	NodesExecutedTotal *prometheus.CounterVec
	NodeDuration       *prometheus.HistogramVec
	NodeErrors         *prometheus.CounterVec

	LLMRequestsTotal   *prometheus.CounterVec
	LLMRequestDuration *prometheus.HistogramVec
	LLMTokensTotal     *prometheus.CounterVec
	LLMErrors          *prometheus.CounterVec

	ToolExecutionsTotal *prometheus.CounterVec
	ToolDuration        *prometheus.HistogramVec
	ToolErrors          *prometheus.CounterVec

	EventsPublishedTotal *prometheus.CounterVec
	EventsConsumedTotal  *prometheus.CounterVec

	DBQueriesTotal      *prometheus.CounterVec
	DBQueryDuration     *prometheus.HistogramVec
	DBConnectionsActive prometheus.Gauge

	WorkersActive        prometheus.Gauge
	WorkerHeartbeats     *prometheus.CounterVec
	TasksDispatched      *prometheus.CounterVec
	TasksClaimed         *prometheus.CounterVec
	LeasesExpired        prometheus.Counter
	OutboxPending        prometheus.Gauge
	OutboxPublished      prometheus.Counter
	ConcurrencyConflicts prometheus.Counter
	ErrorsTotal          *prometheus.CounterVec
	PanicsRecovered      prometheus.Counter
}

func NewMetrics(namespace string) *Metrics {
	if namespace == "" {
		namespace = "duragraph"
	}

	return &Metrics{
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_size_bytes",
				Help:      "HTTP request size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),

		RunsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "runs_total",
				Help:      "Total number of runs created",
			},
			// tenant_id first by convention; assistant_id retained so
			// per-assistant slicing remains possible. Empty tenant_id
			// means "unscoped / single-tenant deployment" — see the
			// comment on the Record* helpers below.
			[]string{"tenant_id", "assistant_id"},
		),
		RunDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "run_duration_seconds",
				Help:      "Run duration in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 12),
			},
			[]string{"tenant_id", "assistant_id", "status"},
		),
		RunsActive: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "runs_active",
				Help:      "Number of currently active runs",
			},
			[]string{"tenant_id"},
		),
		RunStatusTransitions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "run_status_transitions_total",
				Help:      "Total number of run status transitions",
			},
			[]string{"tenant_id", "from_status", "to_status"},
		),

		AssistantsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "assistants_total",
				Help:      "Number of assistants per tenant",
			},
			[]string{"tenant_id"},
		),
		ThreadsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "threads_total",
				Help:      "Number of threads per tenant",
			},
			[]string{"tenant_id"},
		),

		NodesExecutedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "nodes_executed_total",
				Help:      "Total number of nodes executed",
			},
			[]string{"node_type", "status"},
		),
		NodeDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "node_duration_seconds",
				Help:      "Node execution duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"node_type"},
		),
		NodeErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "node_errors_total",
				Help:      "Total number of node execution errors",
			},
			[]string{"node_type", "error_type"},
		),

		LLMRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_requests_total",
				Help:      "Total number of LLM requests",
			},
			[]string{"tenant_id", "provider", "model", "status"},
		),
		LLMRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "llm_request_duration_seconds",
				Help:      "LLM request duration in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10),
			},
			[]string{"tenant_id", "provider", "model"},
		),
		LLMTokensTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_tokens_total",
				Help:      "Total number of LLM tokens used",
			},
			[]string{"tenant_id", "provider", "model", "type"},
		),
		LLMErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_errors_total",
				Help:      "Total number of LLM errors",
			},
			[]string{"tenant_id", "provider", "model", "error_type"},
		),

		ToolExecutionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tool_executions_total",
				Help:      "Total number of tool executions",
			},
			[]string{"tool_name", "status"},
		),
		ToolDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "tool_duration_seconds",
				Help:      "Tool execution duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"tool_name"},
		),
		ToolErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tool_errors_total",
				Help:      "Total number of tool errors",
			},
			[]string{"tool_name", "error_type"},
		),

		EventsPublishedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "events_published_total",
				Help:      "Total number of events published",
			},
			[]string{"event_type"},
		),
		EventsConsumedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "events_consumed_total",
				Help:      "Total number of events consumed",
			},
			[]string{"event_type"},
		),

		DBQueriesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "db_queries_total",
				Help:      "Total number of database queries",
			},
			[]string{"operation", "table"},
		),
		DBQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "db_query_duration_seconds",
				Help:      "Database query duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"operation", "table"},
		),
		DBConnectionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_connections_active",
				Help:      "Number of active database connections",
			},
		),

		WorkersActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "workers_active",
				Help:      "Number of active workers",
			},
		),
		WorkerHeartbeats: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "worker_heartbeats_total",
				Help:      "Total worker heartbeats received",
			},
			[]string{"worker_id"},
		),
		TasksDispatched: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tasks_dispatched_total",
				Help:      "Total tasks dispatched to workers",
			},
			[]string{"graph_id"},
		),
		TasksClaimed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tasks_claimed_total",
				Help:      "Total tasks claimed by workers",
			},
			[]string{"worker_id"},
		),
		LeasesExpired: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "leases_expired_total",
				Help:      "Total number of expired run leases",
			},
		),
		OutboxPending: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_pending",
				Help:      "Number of pending outbox messages",
			},
		),
		OutboxPublished: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_published_total",
				Help:      "Total outbox messages published",
			},
		),
		ConcurrencyConflicts: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "concurrency_conflicts_total",
				Help:      "Total optimistic concurrency conflicts",
			},
		),
		ErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "errors_total",
				Help:      "Total errors by category",
			},
			[]string{"category", "code"},
		),
		PanicsRecovered: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "panics_recovered_total",
				Help:      "Total panics recovered by middleware",
			},
		),
	}
}

func (m *Metrics) RecordHTTPRequest(method, path string, status int, duration time.Duration, reqSize, respSize int) {
	statusStr := strconv.Itoa(status)
	m.HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	m.HTTPRequestSize.WithLabelValues(method, path).Observe(float64(reqSize))
	m.HTTPResponseSize.WithLabelValues(method, path).Observe(float64(respSize))
}

// tenantID convention: callers should pass the request-scoped tenant_id
// resolved via middleware.TenantIDFromCtx (or the equivalent on a
// command struct). Pass empty string ("") when no tenant context exists
// — this is the documented single-tenant deployment mode (engine
// running with MIGRATOR_PLATFORM_ENABLED=false). Mimir queries that
// `sum by (tenant_id)` will surface a single "" series in that case,
// which is the intended behaviour.

func (m *Metrics) RecordRunCreated(tenantID, assistantID string) {
	m.RunsTotal.WithLabelValues(tenantID, assistantID).Inc()
	m.RunsActive.WithLabelValues(tenantID).Inc()
}

func (m *Metrics) RecordRunCompleted(tenantID, assistantID, status string, duration time.Duration) {
	m.RunDuration.WithLabelValues(tenantID, assistantID, status).Observe(duration.Seconds())
	m.RunsActive.WithLabelValues(tenantID).Dec()
}

// IncRunsActive / DecRunsActive expose the per-tenant active-runs gauge
// for callers that don't drive it through RecordRunCreated/Completed
// (e.g. lease recovery on engine restart).
func (m *Metrics) IncRunsActive(tenantID string) { m.RunsActive.WithLabelValues(tenantID).Inc() }
func (m *Metrics) DecRunsActive(tenantID string) { m.RunsActive.WithLabelValues(tenantID).Dec() }

// IncAssistants / DecAssistants are wired into CreateAssistant /
// DeleteAssistant command handlers to keep the per-tenant gauge in
// sync. Drift is acceptable at MVP scale (the gauge is observational,
// not authoritative).
func (m *Metrics) IncAssistants(tenantID string) { m.AssistantsTotal.WithLabelValues(tenantID).Inc() }
func (m *Metrics) DecAssistants(tenantID string) { m.AssistantsTotal.WithLabelValues(tenantID).Dec() }

func (m *Metrics) IncThreads(tenantID string) { m.ThreadsTotal.WithLabelValues(tenantID).Inc() }
func (m *Metrics) DecThreads(tenantID string) { m.ThreadsTotal.WithLabelValues(tenantID).Dec() }

func (m *Metrics) RecordNodeExecution(nodeType, status string, duration time.Duration) {
	m.NodesExecutedTotal.WithLabelValues(nodeType, status).Inc()
	m.NodeDuration.WithLabelValues(nodeType).Observe(duration.Seconds())
}

func (m *Metrics) RecordLLMRequest(tenantID, provider, model, status string, duration time.Duration, promptTokens, completionTokens int) {
	m.LLMRequestsTotal.WithLabelValues(tenantID, provider, model, status).Inc()
	m.LLMRequestDuration.WithLabelValues(tenantID, provider, model).Observe(duration.Seconds())
	m.LLMTokensTotal.WithLabelValues(tenantID, provider, model, "prompt").Add(float64(promptTokens))
	m.LLMTokensTotal.WithLabelValues(tenantID, provider, model, "completion").Add(float64(completionTokens))
}

func (m *Metrics) RecordToolExecution(toolName, status string, duration time.Duration) {
	m.ToolExecutionsTotal.WithLabelValues(toolName, status).Inc()
	m.ToolDuration.WithLabelValues(toolName).Observe(duration.Seconds())
}

func (m *Metrics) RecordError(category, code string) {
	m.ErrorsTotal.WithLabelValues(category, code).Inc()
}
