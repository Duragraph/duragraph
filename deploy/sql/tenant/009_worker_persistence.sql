-- 009_worker_persistence.sql: Persistent worker registry and task assignments
-- Replaces in-memory worker registry with PostgreSQL-backed persistence.
-- Adds task_assignments table for reliable task dispatch with lease-based ownership.

-- Workers table: persistent worker registry
CREATE TABLE IF NOT EXISTS workers (
    id              VARCHAR(255) PRIMARY KEY,
    name            VARCHAR(255) NOT NULL,
    status          VARCHAR(50) NOT NULL DEFAULT 'ready',
    capabilities    JSONB NOT NULL DEFAULT '{}'::jsonb,
    graph_definitions JSONB NOT NULL DEFAULT '[]'::jsonb,
    active_runs     INT NOT NULL DEFAULT 0,
    total_runs      INT NOT NULL DEFAULT 0,
    failed_runs     INT NOT NULL DEFAULT 0,
    last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workers_status ON workers (status);
CREATE INDEX IF NOT EXISTS idx_workers_last_heartbeat ON workers (last_heartbeat_at);
CREATE INDEX IF NOT EXISTS idx_workers_capabilities ON workers USING gin (capabilities);

-- Trigger for updated_at on workers
CREATE OR REPLACE FUNCTION update_workers_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_workers_updated_at ON workers;
CREATE TRIGGER trigger_workers_updated_at
    BEFORE UPDATE ON workers
    FOR EACH ROW
    EXECUTE FUNCTION update_workers_updated_at();

-- Task assignments table: persistent task queue with lease-based ownership
CREATE TABLE IF NOT EXISTS task_assignments (
    id              BIGSERIAL PRIMARY KEY,
    run_id          UUID NOT NULL,
    worker_id       VARCHAR(255),
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    graph_id        VARCHAR(255) NOT NULL,
    thread_id       UUID NOT NULL,
    assistant_id    UUID NOT NULL,
    input           JSONB NOT NULL DEFAULT '{}'::jsonb,
    config          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    retry_count     INT NOT NULL DEFAULT 0,
    max_retries     INT NOT NULL DEFAULT 3,
    error_message   TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_assignments_status ON task_assignments (status);
CREATE INDEX IF NOT EXISTS idx_task_assignments_worker_id ON task_assignments (worker_id);
CREATE INDEX IF NOT EXISTS idx_task_assignments_run_id ON task_assignments (run_id);
CREATE INDEX IF NOT EXISTS idx_task_assignments_graph_id ON task_assignments (graph_id);
CREATE INDEX IF NOT EXISTS idx_task_assignments_lease ON task_assignments (lease_expires_at)
    WHERE status = 'claimed';
CREATE INDEX IF NOT EXISTS idx_task_assignments_pending ON task_assignments (created_at)
    WHERE status = 'pending';

-- Trigger for updated_at on task_assignments
CREATE OR REPLACE FUNCTION update_task_assignments_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_task_assignments_updated_at ON task_assignments;
CREATE TRIGGER trigger_task_assignments_updated_at
    BEFORE UPDATE ON task_assignments
    FOR EACH ROW
    EXECUTE FUNCTION update_task_assignments_updated_at();

-- Add worker tracking columns to runs table
ALTER TABLE runs ADD COLUMN IF NOT EXISTS worker_id VARCHAR(255);
ALTER TABLE runs ADD COLUMN IF NOT EXISTS retry_count INT NOT NULL DEFAULT 0;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS last_heartbeat_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_runs_worker_id ON runs (worker_id);
CREATE INDEX IF NOT EXISTS idx_runs_lease_expires ON runs (lease_expires_at)
    WHERE status IN ('in_progress', 'running') AND lease_expires_at IS NOT NULL;

COMMENT ON TABLE workers IS 'Persistent worker registry replacing in-memory worker state';
COMMENT ON TABLE task_assignments IS 'Task queue for worker dispatch with lease-based ownership and retry support';
COMMENT ON COLUMN task_assignments.lease_expires_at IS 'When the lease expires; expired leases can be reclaimed by other workers';
COMMENT ON COLUMN task_assignments.status IS 'Task status: pending, claimed, completed, failed, expired';
COMMENT ON COLUMN runs.worker_id IS 'ID of the worker executing this run';
COMMENT ON COLUMN runs.lease_expires_at IS 'When the worker lease expires for this run';
