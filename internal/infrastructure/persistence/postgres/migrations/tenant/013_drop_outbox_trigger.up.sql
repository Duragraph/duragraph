-- 013_drop_outbox_trigger.sql: remove the auto_publish_to_outbox
-- trigger now that the EventStore writes the outbox row itself in the
-- same transaction as the event row. See
-- internal/infrastructure/persistence/postgres/event_store.go.
--
-- The increment_version_on_event trigger on the same table is
-- intentionally KEPT — it's stream-version bookkeeping, unrelated to
-- outbox.
--
-- The cleanup_published_outbox() function is KEPT too — the
-- CleanupWorker still calls it on a schedule.

DROP TRIGGER IF EXISTS auto_publish_to_outbox ON events;
DROP FUNCTION IF EXISTS publish_event_to_outbox();
