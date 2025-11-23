package monitoring

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// Run metrics
	RunsTotal            *prometheus.CounterVec
	RunDuration          *prometheus.HistogramVec
	RunsActive           prometheus.Gauge
	RunStatusTransitions *prometheus.CounterVec

	// Graph execution metrics
	NodesExecutedTotal *prometheus.CounterVec
	NodeDuration       *prometheus.HistogramVec
	NodeErrors         *prometheus.CounterVec

	// LLM metrics
	LLMRequestsTotal   *prometheus.CounterVec
	LLMRequestDuration *prometheus.HistogramVec
	LLMTokensTotal     *prometheus.CounterVec
	LLMErrors          *prometheus.CounterVec

	// Tool metrics
	ToolExecutionsTotal *prometheus.CounterVec
	ToolDuration        *prometheus.HistogramVec
	ToolErrors          *prometheus.CounterVec

	// Event bus metrics
	EventsPublishedTotal *prometheus.CounterVec
	EventsConsumedTotal  *prometheus.CounterVec

	// Database metrics
	DBQueriesTotal      *prometheus.CounterVec
	DBQueryDuration     *prometheus.HistogramVec
	DBConnectionsActive prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics(namespace string) *Metrics {
	if namespace == "" {
		namespace = "duragraph"
	}

	return &Metrics{
		// HTTP metrics
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

		// Run metrics
		RunsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "runs_total",
				Help:      "Total number of runs created",
			},
			[]string{"assistant_id"},
		),
		RunDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "run_duration_seconds",
				Help:      "Run duration in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 12),
			},
			[]string{"assistant_id", "status"},
		),
		RunsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "runs_active",
				Help:      "Number of currently active runs",
			},
		),
		RunStatusTransitions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "run_status_transitions_total",
				Help:      "Total number of run status transitions",
			},
			[]string{"from_status", "to_status"},
		),

		// Graph execution metrics
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

		// LLM metrics
		LLMRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_requests_total",
				Help:      "Total number of LLM requests",
			},
			[]string{"provider", "model", "status"},
		),
		LLMRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "llm_request_duration_seconds",
				Help:      "LLM request duration in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10),
			},
			[]string{"provider", "model"},
		),
		LLMTokensTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_tokens_total",
				Help:      "Total number of LLM tokens used",
			},
			[]string{"provider", "model", "type"},
		),
		LLMErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_errors_total",
				Help:      "Total number of LLM errors",
			},
			[]string{"provider", "model", "error_type"},
		),

		// Tool metrics
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

		// Event bus metrics
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

		// Database metrics
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
	}
}

// RecordHTTPRequest records an HTTP request metric
func (m *Metrics) RecordHTTPRequest(method, path string, status int, duration time.Duration, reqSize, respSize int) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, string(rune(status))).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	m.HTTPRequestSize.WithLabelValues(method, path).Observe(float64(reqSize))
	m.HTTPResponseSize.WithLabelValues(method, path).Observe(float64(respSize))
}

// RecordRunCreated records a run creation
func (m *Metrics) RecordRunCreated(assistantID string) {
	m.RunsTotal.WithLabelValues(assistantID).Inc()
	m.RunsActive.Inc()
}

// RecordRunCompleted records a run completion
func (m *Metrics) RecordRunCompleted(assistantID, status string, duration time.Duration) {
	m.RunDuration.WithLabelValues(assistantID, status).Observe(duration.Seconds())
	m.RunsActive.Dec()
}

// RecordNodeExecution records node execution
func (m *Metrics) RecordNodeExecution(nodeType, status string, duration time.Duration) {
	m.NodesExecutedTotal.WithLabelValues(nodeType, status).Inc()
	m.NodeDuration.WithLabelValues(nodeType).Observe(duration.Seconds())
}

// RecordLLMRequest records an LLM request
func (m *Metrics) RecordLLMRequest(provider, model, status string, duration time.Duration, promptTokens, completionTokens int) {
	m.LLMRequestsTotal.WithLabelValues(provider, model, status).Inc()
	m.LLMRequestDuration.WithLabelValues(provider, model).Observe(duration.Seconds())
	m.LLMTokensTotal.WithLabelValues(provider, model, "prompt").Add(float64(promptTokens))
	m.LLMTokensTotal.WithLabelValues(provider, model, "completion").Add(float64(completionTokens))
}

// RecordToolExecution records tool execution
func (m *Metrics) RecordToolExecution(toolName, status string, duration time.Duration) {
	m.ToolExecutionsTotal.WithLabelValues(toolName, status).Inc()
	m.ToolDuration.WithLabelValues(toolName).Observe(duration.Seconds())
}
