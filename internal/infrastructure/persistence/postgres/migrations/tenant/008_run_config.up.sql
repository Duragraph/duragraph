-- 008_run_config.sql: Add config support for runs

-- Add config column to runs table for LangGraph config support
-- Config includes: tags, recursion_limit, configurable fields
ALTER TABLE runs ADD COLUMN IF NOT EXISTS config JSONB DEFAULT '{}'::jsonb;

-- Add multitask_strategy column for concurrent run handling
-- Values: reject (default), interrupt, rollback, enqueue
ALTER TABLE runs ADD COLUMN IF NOT EXISTS multitask_strategy VARCHAR(50) DEFAULT 'reject';

-- Create index for querying by config values
CREATE INDEX IF NOT EXISTS idx_runs_config ON runs USING gin(config);

-- Add tags to runs for filtering
-- Tags can be queried via: config->'tags' ? 'tag_name'
COMMENT ON COLUMN runs.config IS 'LangGraph-compatible config object containing tags, recursion_limit, and configurable fields';
COMMENT ON COLUMN runs.multitask_strategy IS 'Strategy for handling concurrent runs: reject, interrupt, rollback, enqueue';
