-- 004_projections.sql: Materialized views and projections for fast queries

-- Run status view (optimized for status queries)
CREATE MATERIALIZED VIEW IF NOT EXISTS run_status_view AS
SELECT
    r.id,
    r.thread_id,
    r.assistant_id,
    r.status,
    r.created_at,
    r.started_at,
    r.completed_at,
    r.updated_at,
    a.name as assistant_name,
    COUNT(DISTINCT eh.id) as total_nodes_executed,
    COUNT(DISTINCT CASE WHEN eh.status = 'completed' THEN eh.id END) as nodes_completed,
    COUNT(DISTINCT CASE WHEN eh.status = 'failed' THEN eh.id END) as nodes_failed,
    MAX(eh.completed_at) as last_node_completed_at,
    CASE
        WHEN r.completed_at IS NOT NULL AND r.started_at IS NOT NULL
        THEN EXTRACT(EPOCH FROM (r.completed_at - r.started_at)) * 1000
        ELSE NULL
    END as total_duration_ms
FROM runs r
LEFT JOIN assistants a ON r.assistant_id = a.id
LEFT JOIN execution_history eh ON r.id = eh.run_id
GROUP BY r.id, a.name;

CREATE UNIQUE INDEX idx_run_status_view_id ON run_status_view(id);
CREATE INDEX idx_run_status_view_status ON run_status_view(status);
CREATE INDEX idx_run_status_view_created_at ON run_status_view(created_at DESC);
CREATE INDEX idx_run_status_view_thread ON run_status_view(thread_id);

-- Function to refresh materialized view
CREATE OR REPLACE FUNCTION refresh_run_status_view()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY run_status_view;
END;
$$ language 'plpgsql';

-- Event statistics view (for monitoring)
CREATE MATERIALIZED VIEW IF NOT EXISTS event_statistics AS
SELECT
    aggregate_type,
    event_type,
    COUNT(*) as event_count,
    MIN(occurred_at) as first_occurrence,
    MAX(occurred_at) as last_occurrence,
    DATE_TRUNC('hour', occurred_at) as hour_bucket
FROM events
GROUP BY aggregate_type, event_type, DATE_TRUNC('hour', occurred_at);

CREATE INDEX idx_event_statistics_aggregate ON event_statistics(aggregate_type);
CREATE INDEX idx_event_statistics_type ON event_statistics(event_type);
CREATE INDEX idx_event_statistics_hour ON event_statistics(hour_bucket DESC);

-- Outbox health view (monitoring outbox backlog)
CREATE OR REPLACE VIEW outbox_health AS
SELECT
    COUNT(*) FILTER (WHERE NOT published) as pending_count,
    COUNT(*) FILTER (WHERE published) as published_count,
    COUNT(*) FILTER (WHERE NOT published AND attempts > 3) as failed_count,
    MAX(created_at) FILTER (WHERE NOT published) as oldest_pending,
    AVG(attempts) FILTER (WHERE NOT published) as avg_attempts
FROM outbox;

-- Thread activity summary (for thread queries)
CREATE OR REPLACE VIEW thread_summary AS
SELECT
    t.id,
    t.created_at,
    t.updated_at,
    COUNT(DISTINCT m.id) as message_count,
    COUNT(DISTINCT r.id) as run_count,
    MAX(m.created_at) as last_message_at,
    MAX(r.created_at) as last_run_at
FROM threads t
LEFT JOIN messages m ON t.id = m.thread_id
LEFT JOIN runs r ON t.id = r.thread_id
GROUP BY t.id;
