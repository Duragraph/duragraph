-- Run context — built from spec/models/d2/postgres.d2 (run_ctx).
-- Per-tenant database. The Run aggregate plus its interrupts and per-node
-- execution history.

-- runs: aggregate root for one execution instance. version = optimistic
-- concurrency token; lease_epoch = fencing token for worker assignment.
CREATE TABLE runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id    UUID REFERENCES threads (id) ON DELETE CASCADE,  -- nullable: stateless runs have no thread
    assistant_id UUID NOT NULL REFERENCES assistants (id) ON DELETE RESTRICT,
    status       VARCHAR(20) NOT NULL DEFAULT 'queued'
                     CHECK (status IN ('queued', 'in_progress', 'requires_action',
                                       'completed', 'failed', 'cancelled')),
    input        JSONB NOT NULL DEFAULT '{}'::jsonb,
    output       JSONB,
    error        TEXT,
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    kwargs       JSONB NOT NULL DEFAULT '{}'::jsonb,   -- LangGraph run kwargs
    multitask_strategy VARCHAR(20) NOT NULL DEFAULT 'reject'
                     CHECK (multitask_strategy IN ('reject', 'rollback', 'interrupt', 'enqueue')),
    version      INTEGER NOT NULL DEFAULT 0,
    lease_epoch  INTEGER NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_runs_thread ON runs (thread_id);
CREATE INDEX idx_runs_assistant ON runs (assistant_id);
CREATE INDEX idx_runs_status ON runs (status);
CREATE INDEX idx_runs_created_at ON runs (created_at DESC);

CREATE TRIGGER update_runs_updated_at
    BEFORE UPDATE ON runs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- interrupts: human-in-the-loop suspension points on a run.
CREATE TABLE interrupts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id      UUID NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    node_id     VARCHAR(255) NOT NULL,
    reason      VARCHAR(30) NOT NULL
                    CHECK (reason IN ('tool_call', 'approval_required', 'input_needed')),
    state       JSONB NOT NULL DEFAULT '{}'::jsonb,
    tool_calls  JSONB,
    resolved    BOOLEAN NOT NULL DEFAULT FALSE,
    resolved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_interrupts_run ON interrupts (run_id);
CREATE INDEX idx_interrupts_unresolved ON interrupts (run_id) WHERE NOT resolved;

-- execution_history: one row per node execution within a run.
CREATE TABLE execution_history (
    id           BIGSERIAL PRIMARY KEY,
    run_id       UUID NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    node_id      VARCHAR(255) NOT NULL,
    node_type    VARCHAR(20) NOT NULL
                     CHECK (node_type IN ('start', 'end', 'llm', 'tool', 'conditional')),
    status       VARCHAR(20) NOT NULL
                     CHECK (status IN ('started', 'completed', 'failed', 'skipped')),
    input        JSONB,
    output       JSONB,
    error        TEXT,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    duration_ms  INTEGER
);

CREATE INDEX idx_execution_history_run ON execution_history (run_id, started_at);
