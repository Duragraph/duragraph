-- Reverse of 002_event_sourcing (platform).
--
-- update_updated_at_column is deliberately NOT dropped: 001_platform defines
-- the same function and its users/tenants triggers still depend on it.
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS snapshots;
DROP TABLE IF EXISTS events;
DROP TRIGGER IF EXISTS trg_increment_version_on_event ON events;
DROP FUNCTION IF EXISTS increment_version_on_event();
DROP TABLE IF EXISTS event_streams;
