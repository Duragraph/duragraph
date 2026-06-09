-- Worker & Scheduling context — built from spec/models/d2/postgres.d2
-- (worker_ctx). Per-tenant database. Worker registration/heartbeat/dispatch and
-- cron schedules. Also adds the dispatch columns the worker-claim path needs to
-- the runs aggregate.

-- workers: stateless executor registration + heartbeat lease. A worker
-- advertises the graph_ids it can run; the claim path matches queued runs to a
-- worker's graphs. An expired lease_expires_at means the worker is presumed
-- dead and its in-flight runs are requeued.
CREATE TABLE workers (
    worker_id         UUID PRIMARY KEY,
    graphs            TEXT[] NOT NULL DEFAULT '{}',
    capacity          INTEGER NOT NULL DEFAULT 1,
    active_runs       INTEGER NOT NULL DEFAULT 0,
    status            VARCHAR(20) NOT NULL DEFAULT 'online'
                          CHECK (status IN ('online', 'draining', 'offline')),
    lease_expires_at  TIMESTAMPTZ,
    last_heartbeat_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workers_status ON workers (status);
CREATE INDEX idx_workers_graphs ON workers USING GIN (graphs);

CREATE TRIGGER update_workers_updated_at
    BEFORE UPDATE ON workers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- crons: scheduled run definitions. A cron may be thread-scoped (thread_id set)
-- or stateless (thread_id null). The scheduler dispatches when next_run_at passes.
CREATE TABLE crons (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id    UUID REFERENCES threads (id) ON DELETE CASCADE,
    assistant_id UUID NOT NULL REFERENCES assistants (id) ON DELETE CASCADE,
    schedule     VARCHAR(100) NOT NULL,
    input        JSONB NOT NULL DEFAULT '{}'::jsonb,
    config       JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    next_run_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_crons_assistant ON crons (assistant_id);
CREATE INDEX idx_crons_thread ON crons (thread_id);
CREATE INDEX idx_crons_next_run ON crons (next_run_at) WHERE next_run_at IS NOT NULL;
CREATE INDEX idx_crons_metadata ON crons USING GIN (metadata);

CREATE TRIGGER update_crons_updated_at
    BEFORE UPDATE ON crons
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- runs dispatch columns: which worker holds the run, dispatch priority, and the
-- graph_id denormalized from the assistant so the claim query can filter by the
-- worker's advertised graphs without a join.
ALTER TABLE runs ADD COLUMN worker_id UUID REFERENCES workers (worker_id) ON DELETE SET NULL;
ALTER TABLE runs ADD COLUMN priority  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runs ADD COLUMN graph_id  VARCHAR(255);

-- Worker-claim index: queued runs by graph, highest priority / oldest first.
CREATE INDEX idx_runs_claim ON runs (graph_id, priority DESC, created_at)
    WHERE status = 'queued';
CREATE INDEX idx_runs_worker ON runs (worker_id) WHERE worker_id IS NOT NULL;
