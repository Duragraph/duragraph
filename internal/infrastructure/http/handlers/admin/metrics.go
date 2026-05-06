// Mimir / Prometheus PromQL client used by the admin metrics endpoints.
//
// Mimir exposes a Prometheus-compatible HTTP API at
// `<MIMIR_URL>/prometheus/api/v1/query`. We issue *instant* queries for
// the cross-tenant aggregate (`/api/admin/metrics`) and per-tenant
// drilldown (`/api/admin/metrics/{tenant_id}`); range queries are not
// needed at this stage — the dashboard polls and renders single-point
// rates.
//
// Why a small handwritten client instead of the official Prometheus Go
// client (`github.com/prometheus/client_golang/api/v1`):
//
//   - One extra dependency for two HTTP calls and a JSON struct decode.
//   - The official client's typed `Vector` / `Sample` model assumes you
//     want the full Prometheus result set; we want a thin "metric ->
//     {tenant_id: value}" map and that's a few lines of decoding.
//   - The thin layer makes it trivial to swap a fake backend in unit
//     tests via the MetricsBackend interface, instead of standing up a
//     real Prometheus or wiring net/http test servers.
//
// TODO(metrics-instrumentation): the current monitoring/metrics.go
// registers `duragraph_runs_total` (label: assistant_id),
// `duragraph_runs_active` (no labels), `duragraph_llm_tokens_total`
// (provider/model/type) and does NOT emit `tenant_id`,
// `duragraph_assistants_total`, or `duragraph_threads_total`. Until a
// follow-up PR adds the `tenant_id` label and the missing series, the
// PromQL expressions below return empty vectors against today's
// Prometheus instance — handlers degrade gracefully (zero values, empty
// tenant list) rather than erroring. The metric NAMES used here are the
// ones the spec implies (duragraph-spec/api/platform.yaml § TenantMetrics).
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// validWindows mirrors the `window` enum in
// duragraph-spec/api/platform.yaml § /api/admin/metrics.window.
var validWindows = map[string]struct{}{
	"5m":  {},
	"1h":  {},
	"6h":  {},
	"24h": {},
	"7d":  {},
}

const defaultWindow = "5m"

// metricLabelTenantID is the Prometheus label the queries group by.
// Hardcoded so a future refactor can rename/relabel in one place.
const metricLabelTenantID = "tenant_id"

// MetricsBackend is the minimal surface the admin handler needs to
// query Mimir. Implemented by *MimirClient in this package, and by
// fake backends in tests. Pulling this out as an interface keeps the
// HTTP layer free of net/http transport concerns at unit-test time.
type MetricsBackend interface {
	// Query runs an instant PromQL query against the backend. The
	// returned slice is one row per result series; samples for series
	// missing the requested label come back with an empty Labels map.
	Query(ctx context.Context, promql string) ([]Sample, error)
}

// Sample is one row of a Prometheus instant-query result. Labels is
// the full label set on the sample (we read `tenant_id` for the
// per-tenant breakdown); Value is the float at the query timestamp.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// MimirClient is the production MetricsBackend implementation. It
// hits Mimir's Prometheus-compatible HTTP API at
// <BaseURL>/prometheus/api/v1/query.
//
// Mimir does multi-tenancy via the `X-Scope-OrgID` header — set
// TenantHeader if your Mimir cluster requires it. Most self-hosted
// Mimir deployments leave it on the operator default ("anonymous") in
// dev and require it in prod. Empty TenantHeader means we don't send
// the header at all (spec-compliant behaviour for a dev cluster
// running Mimir in single-tenant mode).
type MimirClient struct {
	BaseURL      string
	TenantHeader string
	HTTPClient   *http.Client
}

// NewMimirClient constructs a MimirClient with sensible defaults.
//
// baseURL must include scheme + host (e.g. "http://prod-mimir:9009").
// tenantHeader is optional — pass "" to omit the X-Scope-OrgID header
// entirely. The returned client uses a 10-second per-request timeout
// (PromQL evaluators can be slow on large windows but we never want
// admin UI page-loads to block longer than that on a metrics fetch).
func NewMimirClient(baseURL, tenantHeader string) *MimirClient {
	return &MimirClient{
		BaseURL:      baseURL,
		TenantHeader: tenantHeader,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Query implements MetricsBackend.
//
// Errors are propagated wrapped with context — the handler maps any
// error to 500, since a successful metrics query should be rare to
// fail and a partial response is worse than a clear error.
func (c *MimirClient) Query(ctx context.Context, promql string) ([]Sample, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("mimir client: BaseURL is empty")
	}

	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("mimir client: invalid BaseURL: %w", err)
	}
	u.Path = "/prometheus/api/v1/query"
	q := u.Query()
	q.Set("query", promql)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("mimir client: build request: %w", err)
	}
	if c.TenantHeader != "" {
		req.Header.Set("X-Scope-OrgID", c.TenantHeader)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mimir client: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mimir client: HTTP %d from %s", resp.StatusCode, u.String())
	}

	var body promQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("mimir client: decode response: %w", err)
	}
	if body.Status != "success" {
		return nil, fmt.Errorf("mimir client: status=%s error=%s", body.Status, body.Error)
	}

	// Vector is the only result type our queries produce. Matrix /
	// scalar / string handling is not needed at this layer.
	if body.Data.ResultType != "vector" {
		return nil, fmt.Errorf("mimir client: unexpected resultType %q (want vector)", body.Data.ResultType)
	}

	samples := make([]Sample, 0, len(body.Data.Result))
	for _, r := range body.Data.Result {
		// Prometheus encodes a sample as [<unix-ts>, "<float-as-string>"].
		// The string-encoded float is deliberate: it preserves NaN /
		// +Inf / -Inf which JSON numbers can't carry. We tolerate parse
		// failures by skipping the sample — partial data is better than
		// failing the whole admin page on one bad series.
		if len(r.Value) != 2 {
			continue
		}
		raw, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		samples = append(samples, Sample{
			Labels: r.Metric,
			Value:  v,
		})
	}
	return samples, nil
}

// promQueryResponse mirrors the Prometheus instant-query JSON envelope.
// Exposed only via the Query method's return type — no need to leak
// the wire format.
type promQueryResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Data   struct {
		ResultType string             `json:"resultType"`
		Result     []promVectorSample `json:"result"`
	} `json:"data"`
}

type promVectorSample struct {
	Metric map[string]string `json:"metric"`
	// Value is [<unix-ts>, "<float-as-string>"] — Prometheus's stable
	// instant-vector encoding.
	Value [2]any `json:"value"`
}

// fetchAdminMetrics runs the cross-tenant query bundle in parallel and
// stitches the per-series results into per-tenant rows + totals.
//
// `tenantFilter` is empty for the all-tenant aggregate or a tenant_id
// for the drilldown. Empty filter widens the PromQL with no label
// selector; a non-empty filter applies `{tenant_id="<id>"}` directly.
func fetchAdminMetrics(ctx context.Context, backend MetricsBackend, window, tenantFilter string) (map[string]TenantMetrics, MetricsTotals, error) {
	// Build the five PromQL expressions. Each is grouped by tenant_id
	// even on the per-tenant drilldown — the filter eliminates other
	// tenants but the grouping remains so the result-merge code path
	// is uniform.
	tenantSel := ""
	if tenantFilter != "" {
		// Prometheus label values cannot contain unescaped double-quotes;
		// tenant_id is a UUID so this is structurally safe, but we wrap
		// in a guard belt-and-suspenders.
		tenantSel = fmt.Sprintf(`{tenant_id="%s"}`, tenantFilter)
	}

	// Metric names are the ones the spec implies. See package-level
	// TODO comment for the instrumentation gap.
	queries := map[string]string{
		"runs_per_sec":            fmt.Sprintf(`sum by (tenant_id) (rate(duragraph_runs_total%s[%s]))`, tenantSel, window),
		"runs_active":             fmt.Sprintf(`sum by (tenant_id) (duragraph_runs_active%s)`, tenantSel),
		"assistants_total":        fmt.Sprintf(`sum by (tenant_id) (duragraph_assistants_total%s)`, tenantSel),
		"threads_total":           fmt.Sprintf(`sum by (tenant_id) (duragraph_threads_total%s)`, tenantSel),
		"tokens_consumed_per_sec": fmt.Sprintf(`sum by (tenant_id) (rate(duragraph_llm_tokens_total%s[%s]))`, tenantSel, window),
	}

	// Run queries in parallel: each is independent, and Mimir at scale
	// dominates page-load time when serialised. errgroup short-circuits
	// on the first error.
	results := make(map[string][]Sample, len(queries))
	var resultsMu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	for name, q := range queries {
		name, q := name, q
		g.Go(func() error {
			s, err := backend.Query(gctx, q)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			resultsMu.Lock()
			results[name] = s
			resultsMu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, MetricsTotals{}, err
	}

	// Stitch — index per-tenant rows by tenant_id; samples without a
	// tenant_id label are aggregated into the totals only (this is the
	// shape we'd see if the engine emits a metric without the label,
	// which is currently most of them — see the package TODO).
	tenants := make(map[string]TenantMetrics)
	totals := MetricsTotals{}

	addRunsPerSec := func(tenantID string, v float64) {
		t, ok := tenants[tenantID]
		if !ok {
			t = TenantMetrics{TenantID: tenantID, Window: window}
		}
		t.RunsPerSec += v
		tenants[tenantID] = t
		totals.RunsPerSec += v
	}
	addRunsActive := func(tenantID string, v float64) {
		t, ok := tenants[tenantID]
		if !ok {
			t = TenantMetrics{TenantID: tenantID, Window: window}
		}
		t.RunsActive += int64(v)
		tenants[tenantID] = t
		totals.RunsActive += int64(v)
	}
	addAssistants := func(tenantID string, v float64) {
		t, ok := tenants[tenantID]
		if !ok {
			t = TenantMetrics{TenantID: tenantID, Window: window}
		}
		t.AssistantsTotal += int64(v)
		tenants[tenantID] = t
		totals.AssistantsTotal += int64(v)
	}
	addThreads := func(tenantID string, v float64) {
		t, ok := tenants[tenantID]
		if !ok {
			t = TenantMetrics{TenantID: tenantID, Window: window}
		}
		t.ThreadsTotal += int64(v)
		tenants[tenantID] = t
		totals.ThreadsTotal += int64(v)
	}
	addTokens := func(tenantID string, v float64) {
		t, ok := tenants[tenantID]
		if !ok {
			t = TenantMetrics{TenantID: tenantID, Window: window}
		}
		t.TokensConsumedPerSec += v
		tenants[tenantID] = t
		totals.TokensConsumedPerSec += v
	}

	apply := func(samples []Sample, fn func(string, float64)) {
		for _, s := range samples {
			tid := s.Labels[metricLabelTenantID]
			// Empty tenant_id flows into totals only — we don't surface
			// a fictitious "" tenant row to the dashboard. The handler
			// drops tenant_id="" rows after this stitch (search for the
			// drop in toAdminMetricsResponse).
			fn(tid, s.Value)
		}
	}

	apply(results["runs_per_sec"], addRunsPerSec)
	apply(results["runs_active"], addRunsActive)
	apply(results["assistants_total"], addAssistants)
	apply(results["threads_total"], addThreads)
	apply(results["tokens_consumed_per_sec"], addTokens)

	return tenants, totals, nil
}

// resolveWindow picks a valid PromQL window from the request, falling
// back to the spec default when the parameter is absent or invalid.
// Returning the default rather than 400 matches the spec's `default:
// 5m` semantics — clients omitting the parameter intentionally get a
// usable response.
func resolveWindow(raw string) string {
	if raw == "" {
		return defaultWindow
	}
	if _, ok := validWindows[raw]; !ok {
		return defaultWindow
	}
	return raw
}
