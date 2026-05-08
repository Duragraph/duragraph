-- 012_crons.sql: Cron job scheduling for LangGraph Crons API compatibility
-- Provides scheduled run execution with cron expressions, timezone support, and TTL.

CREATE TABLE IF NOT EXISTS crons (
    cron_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assistant_id TEXT    NOT NULL,
    thread_id    TEXT,
    schedule     TEXT    NOT NULL,
    timezone     TEXT    NOT NULL DEFAULT 'UTC',
    payload      JSONB   NOT NULL DEFAULT '{}',
    metadata     JSONB   NOT NULL DEFAULT '{}',
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    on_run_completed TEXT NOT NULL DEFAULT 'keep',
    end_time     TIMESTAMPTZ,
    next_run_date TIMESTAMPTZ,
    user_id      TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_crons_assistant_id ON crons (assistant_id);
CREATE INDEX IF NOT EXISTS idx_crons_thread_id ON crons (thread_id) WHERE thread_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_crons_enabled ON crons (enabled, next_run_date) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_crons_next_run ON crons (next_run_date) WHERE enabled = TRUE AND next_run_date IS NOT NULL;

COMMENT ON TABLE crons IS 'LangGraph-compatible cron job definitions for scheduled runs';
COMMENT ON COLUMN crons.schedule IS 'Cron expression: minute hour day-of-month month day-of-week';
COMMENT ON COLUMN crons.timezone IS 'IANA timezone for schedule interpretation (default UTC)';
COMMENT ON COLUMN crons.payload IS 'Run configuration: input, config, metadata, webhook, interrupt settings';
COMMENT ON COLUMN crons.on_run_completed IS 'Cleanup behavior for stateless crons: delete or keep thread';
COMMENT ON COLUMN crons.next_run_date IS 'Computed next execution time based on schedule and timezone';
