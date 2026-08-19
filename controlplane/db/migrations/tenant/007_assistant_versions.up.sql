-- Assistant version history — LangGraph-Cloud assistant versioning. Per-tenant
-- database. Each assistant carries a monotonic integer `version`; every create
-- and every update snapshots the assistant's full editable state into
-- assistant_versions keyed by (assistant_id, version). The live assistants row
-- holds the CURRENTLY ACTIVE version (which set_latest may re-point to an older
-- snapshot); MAX(assistant_versions.version) is the highest version ever minted,
-- so a subsequent update always mints MAX+1 rather than colliding after a
-- rollback. POST /assistants/{id}/versions reads this table; POST
-- /assistants/{id}/latest copies a chosen snapshot back onto the live row.
--
-- Columns mirror the editable assistants columns (minus timestamps' update
-- trigger — a snapshot is immutable, so only created_at is recorded).
CREATE TABLE assistant_versions (
    assistant_id UUID    NOT NULL REFERENCES assistants (id) ON DELETE CASCADE,
    version      INTEGER NOT NULL,
    graph_id     VARCHAR(255),
    name         VARCHAR(255) NOT NULL,
    description  TEXT,
    model        VARCHAR(100),
    instructions TEXT,
    tools        JSONB NOT NULL DEFAULT '[]'::jsonb,
    config       JSONB NOT NULL DEFAULT '{}'::jsonb,
    context      JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (assistant_id, version)
);

-- Newest-first version listing (POST /assistants/{id}/versions ORDER BY version DESC).
CREATE INDEX idx_assistant_versions_desc ON assistant_versions (assistant_id, version DESC);

-- metadata filtering on the versions search request.
CREATE INDEX idx_assistant_versions_metadata ON assistant_versions USING GIN (metadata);

-- Backfill: snapshot every existing assistant at its current active version, so
-- the history table is never behind the live rows after this migration lands.
INSERT INTO assistant_versions
    (assistant_id, version, graph_id, name, description, model, instructions, tools, config, context, metadata)
SELECT id, version, graph_id, name, description, model, instructions, tools, config, context, metadata
FROM assistants
ON CONFLICT (assistant_id, version) DO NOTHING;

-- Post-state assertion: every assistant must now have a snapshot at its active
-- version. If the backfill left any assistant without one, fail the migration
-- rather than ship a half-populated history table.
DO $$
DECLARE
    missing INTEGER;
BEGIN
    SELECT count(*) INTO missing
    FROM assistants a
    WHERE NOT EXISTS (
        SELECT 1 FROM assistant_versions av
        WHERE av.assistant_id = a.id AND av.version = a.version
    );
    IF missing > 0 THEN
        RAISE EXCEPTION 'assistant_versions backfill incomplete: % assistant(s) lack a snapshot at their active version', missing;
    END IF;
END $$;
