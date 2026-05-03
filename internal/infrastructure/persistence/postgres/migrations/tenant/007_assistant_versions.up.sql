-- Assistant versioning tables for LangGraph API compatibility
-- Enables assistant version management and schema retrieval

-- Add versioning fields to assistants table
ALTER TABLE assistants ADD COLUMN IF NOT EXISTS version INTEGER DEFAULT 1;
ALTER TABLE assistants ADD COLUMN IF NOT EXISTS graph_id VARCHAR(255);
ALTER TABLE assistants ADD COLUMN IF NOT EXISTS context JSONB DEFAULT '[]';

-- Assistant versions table for version history
CREATE TABLE IF NOT EXISTS assistant_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assistant_id UUID NOT NULL REFERENCES assistants(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    graph_id VARCHAR(255),
    config JSONB NOT NULL DEFAULT '{}',
    context JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(assistant_id, version)
);

-- Indexes for efficient lookups
CREATE INDEX IF NOT EXISTS idx_assistant_versions_assistant_id ON assistant_versions(assistant_id);
CREATE INDEX IF NOT EXISTS idx_assistant_versions_created_at ON assistant_versions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_assistants_graph_id ON assistants(graph_id);
