-- Workflow context — built from spec/models/d2/postgres.d2 (workflow_ctx).
-- Per-tenant database. Assistants, threads, messages, graphs.

-- assistants: binding from public name → a graph (by graph_id name today;
-- content-addressed graph_hash is tracked separately in known-issues).
CREATE TABLE assistants (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    graph_id     VARCHAR(255),
    name         VARCHAR(255) NOT NULL,
    description  TEXT,
    model        VARCHAR(100),
    instructions TEXT,
    tools        JSONB NOT NULL DEFAULT '[]'::jsonb,
    config       JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_assistants_graph_id ON assistants (graph_id);
CREATE INDEX idx_assistants_name ON assistants (name);
CREATE INDEX idx_assistants_metadata ON assistants USING GIN (metadata);

CREATE TRIGGER update_assistants_updated_at
    BEFORE UPDATE ON assistants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- threads: agent-agnostic conversation container. Data is immutable; the
-- assistant link is per-run, not per-thread.
CREATE TABLE threads (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metadata   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_threads_metadata ON threads USING GIN (metadata);

CREATE TRIGGER update_threads_updated_at
    BEFORE UPDATE ON threads
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- messages: append-only conversation entries within a thread.
CREATE TABLE messages (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id  UUID NOT NULL REFERENCES threads (id) ON DELETE CASCADE,
    role       VARCHAR(20) NOT NULL
                   CHECK (role IN ('user', 'assistant', 'system', 'tool')),
    content    TEXT,
    metadata   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_thread ON messages (thread_id, created_at);

-- graphs: workflow definition (nodes + edges). name is the graph_id from the
-- SDK's langgraph.json; assistants resolve to a graph by that name.
CREATE TABLE graphs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assistant_id UUID REFERENCES assistants (id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    version     VARCHAR(50),
    description TEXT,
    nodes       JSONB NOT NULL DEFAULT '[]'::jsonb,
    edges       JSONB NOT NULL DEFAULT '[]'::jsonb,
    config      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_graphs_name ON graphs (name);
CREATE INDEX idx_graphs_assistant ON graphs (assistant_id);

CREATE TRIGGER update_graphs_updated_at
    BEFORE UPDATE ON graphs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
