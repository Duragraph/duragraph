-- Store context — built from spec/models/d2/postgres.d2 (store_ctx).
-- Per-tenant database. Cross-thread KV memory (agent long-term memory + A2A
-- shared state). Sits outside the event-sourced aggregate DAG.
--
-- NOTE: postgres.d2 specifies a surrogate `id bigserial` PK with the natural
-- (namespace, key) as a UNIQUE constraint — NOT a composite PK. Built to the
-- diagram. (The prior implementation used a composite PK + expires_at; both
-- were dropped to match the d2 source of truth.)

-- store_items: namespaced key/value entries. namespace is a hierarchical path.
CREATE TABLE store_items (
    id         BIGSERIAL PRIMARY KEY,
    namespace  TEXT[] NOT NULL,
    key        TEXT NOT NULL,
    value      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (namespace, key)
);

CREATE INDEX idx_store_items_namespace ON store_items USING GIN (namespace);
CREATE INDEX idx_store_items_value ON store_items USING GIN (value jsonb_path_ops);

CREATE TRIGGER update_store_items_updated_at
    BEFORE UPDATE ON store_items
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- store_namespaces: denormalized namespace registry + item_count, for fast
-- namespace listing without scanning store_items.
CREATE TABLE store_namespaces (
    namespace  TEXT[] PRIMARY KEY,
    item_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
