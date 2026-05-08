-- 010_horizontal_scaling.sql: Horizontal scaling safety improvements
-- Adds optimistic concurrency control and lease epoch fencing for multi-instance deployment.

-- Add version column for optimistic concurrency control on run updates
ALTER TABLE runs ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

-- Add lease_epoch for fencing tokens to prevent stale worker writes
ALTER TABLE runs ADD COLUMN IF NOT EXISTS lease_epoch INT NOT NULL DEFAULT 0;

-- Index for expired lease monitoring with advisory lock coordination
CREATE INDEX IF NOT EXISTS idx_runs_expired_leases ON runs (lease_expires_at)
    WHERE status IN ('in_progress', 'running')
      AND lease_expires_at IS NOT NULL;

COMMENT ON COLUMN runs.version IS 'Optimistic concurrency control version; incremented on every update';
COMMENT ON COLUMN runs.lease_epoch IS 'Fencing token incremented on lease reassignment; prevents stale worker writes';
