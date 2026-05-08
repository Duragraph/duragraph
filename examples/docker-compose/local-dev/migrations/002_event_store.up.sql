-- 002_event_store.sql: Event Sourcing tables

-- Event streams table (aggregate metadata)
CREATE TABLE IF NOT EXISTS event_streams (
    stream_id UUID PRIMARY KEY,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id UUID NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(aggregate_type, aggregate_id)
);

CREATE INDEX idx_event_streams_aggregate ON event_streams(aggregate_type, aggregate_id);

-- Events table (event store - append only)
CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    event_id UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    stream_id UUID NOT NULL REFERENCES event_streams(stream_id) ON DELETE CASCADE,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    event_version INTEGER NOT NULL,
    payload JSONB NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(stream_id, event_version)
);

CREATE INDEX idx_events_stream_id ON events(stream_id, event_version);
CREATE INDEX idx_events_aggregate ON events(aggregate_type, aggregate_id);
CREATE INDEX idx_events_event_type ON events(event_type);
CREATE INDEX idx_events_occurred_at ON events(occurred_at DESC);

-- Snapshots table (for performance - rebuild state faster)
CREATE TABLE IF NOT EXISTS snapshots (
    id BIGSERIAL PRIMARY KEY,
    stream_id UUID NOT NULL REFERENCES event_streams(stream_id) ON DELETE CASCADE,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id UUID NOT NULL,
    version INTEGER NOT NULL,
    state JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(stream_id, version)
);

CREATE INDEX idx_snapshots_stream ON snapshots(stream_id, version DESC);
CREATE INDEX idx_snapshots_aggregate ON snapshots(aggregate_type, aggregate_id);

-- Function to increment stream version
CREATE OR REPLACE FUNCTION increment_stream_version()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE event_streams
    SET version = version + 1,
        updated_at = NOW()
    WHERE stream_id = NEW.stream_id;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER increment_version_on_event
AFTER INSERT ON events
FOR EACH ROW
EXECUTE FUNCTION increment_stream_version();
