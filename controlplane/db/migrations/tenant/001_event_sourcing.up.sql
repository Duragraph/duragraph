-- Event-sourcing infrastructure — built from spec/models/d2/postgres.d2
-- (eventsourcing context). Per-tenant database. This is the foundation the
-- write side of every aggregate sits on: append-only events + transactional
-- outbox. Source of truth: postgres.d2 + outbox/transactional-outbox.yml.

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- gen_random_uuid()

-- Shared updated_at trigger function (tenant DB scope).
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- event_streams: one row per aggregate instance. version is the current
-- stream version, auto-advanced by the increment_version_on_event trigger.
CREATE TABLE event_streams (
    stream_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id   UUID NOT NULL,
    version        INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (aggregate_type, aggregate_id)
);

CREATE INDEX idx_event_streams_aggregate ON event_streams (aggregate_type, aggregate_id);

-- events: append-only domain log. event_id is app-generated (UUID), written
-- in the same TX as the outbox row that mirrors it. Never UPDATE or DELETE.
CREATE TABLE events (
    id             BIGSERIAL PRIMARY KEY,
    event_id       UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    stream_id      UUID NOT NULL REFERENCES event_streams (stream_id) ON DELETE CASCADE,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id   UUID NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    event_version  INTEGER NOT NULL,
    payload        JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_stream ON events (stream_id, event_version);
CREATE INDEX idx_events_aggregate ON events (aggregate_type, aggregate_id);
CREATE INDEX idx_events_type ON events (event_type);
CREATE INDEX idx_events_occurred_at ON events (occurred_at);

-- Advance the owning stream's version on every appended event. This is the
-- ONLY trigger in the event path — the outbox is written by application code
-- (no auto_publish_to_outbox trigger; see transactional-outbox.yml).
CREATE OR REPLACE FUNCTION increment_version_on_event()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE event_streams
       SET version = NEW.event_version,
           updated_at = now()
     WHERE stream_id = NEW.stream_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_increment_version_on_event
    AFTER INSERT ON events
    FOR EACH ROW EXECUTE FUNCTION increment_version_on_event();

-- snapshots: periodic serialized aggregate state to bound replay cost.
CREATE TABLE snapshots (
    id             BIGSERIAL PRIMARY KEY,
    stream_id      UUID NOT NULL REFERENCES event_streams (stream_id) ON DELETE CASCADE,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id   UUID NOT NULL,
    version        INTEGER NOT NULL,
    state          JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_snapshots_stream ON snapshots (stream_id, version DESC);

-- outbox: mirrors each event for reliable publish to NATS. event_id matches
-- events.event_id (written same TX). The relay drains WHERE NOT published AND
-- (next_retry_at IS NULL OR next_retry_at <= now()), publishes with
-- Nats-Msg-Id = event_id, then marks published / failed (attempts++,
-- next_retry_at = exponential backoff).
CREATE TABLE outbox (
    id             BIGSERIAL PRIMARY KEY,
    event_id       UUID UNIQUE NOT NULL,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id   UUID NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
    published      BOOLEAN NOT NULL DEFAULT FALSE,
    published_at   TIMESTAMPTZ,
    attempts       INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT,
    next_retry_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Drain index: only unpublished rows, ordered by id for FIFO publish.
CREATE INDEX idx_outbox_unpublished ON outbox (id)
    WHERE NOT published;
