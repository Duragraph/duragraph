-- 003_outbox.sql: Outbox Pattern for reliable event publishing

-- Outbox table (transactional outbox for event publishing)
CREATE TABLE IF NOT EXISTS outbox (
    id BIGSERIAL PRIMARY KEY,
    event_id UUID UNIQUE NOT NULL,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    published_at TIMESTAMP WITH TIME ZONE,
    published BOOLEAN NOT NULL DEFAULT FALSE,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    next_retry_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_outbox_published ON outbox(published, next_retry_at) WHERE NOT published;
CREATE INDEX idx_outbox_event_id ON outbox(event_id);
CREATE INDEX idx_outbox_created_at ON outbox(created_at DESC);
CREATE INDEX idx_outbox_aggregate ON outbox(aggregate_type, aggregate_id);

-- Function to auto-populate outbox from events
CREATE OR REPLACE FUNCTION publish_event_to_outbox()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO outbox (
        event_id,
        aggregate_type,
        aggregate_id,
        event_type,
        payload,
        metadata
    ) VALUES (
        NEW.event_id,
        NEW.aggregate_type,
        NEW.aggregate_id,
        NEW.event_type,
        NEW.payload,
        NEW.metadata
    );
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER auto_publish_to_outbox
AFTER INSERT ON events
FOR EACH ROW
EXECUTE FUNCTION publish_event_to_outbox();

-- Function to cleanup old published events (optional, for maintenance)
CREATE OR REPLACE FUNCTION cleanup_published_outbox(retention_days INTEGER DEFAULT 7)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM outbox
    WHERE published = TRUE
      AND published_at < NOW() - (retention_days || ' days')::INTERVAL;

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ language 'plpgsql';

-- Example usage: SELECT cleanup_published_outbox(7); -- cleanup events older than 7 days
