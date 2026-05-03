-- 005_langgraph_compat.sql: LangGraph Cloud API compatibility updates

-- Update run status values to LangGraph-compatible values
-- Old: completed -> success, failed -> error
-- New: pending, running, success, error, timeout, interrupted, cancelled, requires_action

-- Add kwargs column to runs table
ALTER TABLE runs ADD COLUMN IF NOT EXISTS kwargs JSONB DEFAULT '{}'::jsonb;

-- Update existing status values to LangGraph-compatible values
UPDATE runs SET status = 'success' WHERE status = 'completed';
UPDATE runs SET status = 'error' WHERE status = 'failed';
UPDATE runs SET status = 'pending' WHERE status = 'queued';
UPDATE runs SET status = 'running' WHERE status = 'in_progress';

-- Update runs table comment
COMMENT ON COLUMN runs.status IS 'Run status: pending, running, success, error, timeout, interrupted, cancelled, requires_action';

-- Add new columns to assistants table for LangGraph compatibility
ALTER TABLE assistants ADD COLUMN IF NOT EXISTS graph_id VARCHAR(255);
ALTER TABLE assistants ADD COLUMN IF NOT EXISTS config JSONB DEFAULT '{}'::jsonb;
ALTER TABLE assistants ADD COLUMN IF NOT EXISTS context JSONB DEFAULT '[]'::jsonb;
ALTER TABLE assistants ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;

-- Create index on graph_id for efficient lookups
CREATE INDEX IF NOT EXISTS idx_assistants_graph_id ON assistants(graph_id);

-- Add status column to threads table (LangGraph uses thread status)
ALTER TABLE threads ADD COLUMN IF NOT EXISTS status VARCHAR(50) NOT NULL DEFAULT 'idle';
COMMENT ON COLUMN threads.status IS 'Thread status: idle, busy, interrupted, error';

-- Add values column to threads for storing current state values
ALTER TABLE threads ADD COLUMN IF NOT EXISTS values JSONB DEFAULT '{}'::jsonb;
