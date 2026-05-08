-- 011_store.sql: Key-value store for LangGraph Store API compatibility
-- Provides namespaced key-value storage with JSONB values, TTL support, and search capabilities.

CREATE TABLE IF NOT EXISTS store_items (
    namespace TEXT[] NOT NULL,
    key       TEXT   NOT NULL,
    value     JSONB  NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    PRIMARY KEY (namespace, key)
);

CREATE INDEX IF NOT EXISTS idx_store_items_namespace_prefix
    ON store_items USING GIN (namespace);

CREATE INDEX IF NOT EXISTS idx_store_items_value
    ON store_items USING GIN (value jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_store_items_expires_at
    ON store_items (expires_at)
    WHERE expires_at IS NOT NULL;

COMMENT ON TABLE store_items IS 'LangGraph-compatible namespaced key-value store';
COMMENT ON COLUMN store_items.namespace IS 'Hierarchical namespace as text array (e.g. {documents,user123})';
COMMENT ON COLUMN store_items.key IS 'Item key within namespace';
COMMENT ON COLUMN store_items.value IS 'Item value as JSONB';
COMMENT ON COLUMN store_items.expires_at IS 'Optional TTL expiration timestamp';
