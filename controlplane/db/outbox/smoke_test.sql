-- Functional smoke test for the event-sourcing + transactional-outbox path.
-- Exercises: stream upsert, event append, version-increment trigger, outbox
-- enqueue, drain eligibility, MarkFailed backoff, MarkPublished. Pure DDL/DML
-- against the tenant schema — no app code. Run against a fresh tenant DB.
\set ON_ERROR_STOP on

DO $$
DECLARE
    sid  UUID := gen_random_uuid();
    aid  UUID := gen_random_uuid();
    eid  UUID := gen_random_uuid();
    v    INTEGER;
    n    INTEGER;
BEGIN
    -- write path (what one Save() TX does)
    INSERT INTO event_streams (stream_id, aggregate_type, aggregate_id, version)
        VALUES (sid, 'Run', aid, 0);

    INSERT INTO events (event_id, stream_id, aggregate_type, aggregate_id,
                        event_type, event_version, payload, metadata)
        VALUES (eid, sid, 'Run', aid, 'run.created', 1, '{"x":1}', '{}');

    -- trigger must have advanced the stream version to the event's version
    SELECT version INTO v FROM event_streams WHERE stream_id = sid;
    ASSERT v = 1, format('expected stream version 1, got %s', v);

    INSERT INTO outbox (event_id, aggregate_type, aggregate_id, event_type, payload, metadata)
        VALUES (eid, 'Run', aid, 'run.created', '{"x":1}', '{}');

    -- duplicate enqueue is a no-op (idempotent write)
    INSERT INTO outbox (event_id, aggregate_type, aggregate_id, event_type, payload, metadata)
        VALUES (eid, 'Run', aid, 'run.created', '{"x":1}', '{}')
        ON CONFLICT (event_id) DO NOTHING;
    SELECT count(*) INTO n FROM outbox WHERE event_id = eid;
    ASSERT n = 1, format('expected 1 outbox row after dup enqueue, got %s', n);

    -- drain: row is eligible (unpublished, no next_retry_at)
    SELECT count(*) INTO n FROM outbox
        WHERE NOT published AND (next_retry_at IS NULL OR next_retry_at <= now());
    ASSERT n = 1, format('expected 1 drainable row, got %s', n);

    -- MarkFailed: attempts++, scheduled into the future -> no longer drainable
    UPDATE outbox SET attempts = attempts + 1, last_error = 'nats down',
                      next_retry_at = now() + interval '2 minutes'
        WHERE event_id = eid;
    SELECT count(*) INTO n FROM outbox
        WHERE NOT published AND (next_retry_at IS NULL OR next_retry_at <= now());
    ASSERT n = 0, format('expected 0 drainable rows during backoff, got %s', n);

    -- MarkPublished: terminal
    UPDATE outbox SET published = TRUE, published_at = now(), last_error = NULL
        WHERE event_id = eid;
    SELECT count(*) INTO n FROM outbox WHERE event_id = eid AND published;
    ASSERT n = 1, 'expected row to be published';

    RAISE NOTICE 'OUTBOX SMOKE TEST PASSED';
END $$;
