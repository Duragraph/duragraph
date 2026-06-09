DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS snapshots;
DROP TRIGGER IF EXISTS trg_increment_version_on_event ON events;
DROP FUNCTION IF EXISTS increment_version_on_event();
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS event_streams;
-- update_updated_at_column() is shared; dropped by the last tenant migration.
