-- Reverse of 013_drop_outbox_trigger.up.sql: recreate the trigger.
-- Policy is forward-only — this down migration exists for the
-- migrator's required-pair convention only; do not run it.

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
