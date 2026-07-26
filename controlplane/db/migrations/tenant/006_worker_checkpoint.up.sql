-- Worker checkpoint idempotency — built from spec/models/d2 (worker execution
-- path). A checkpoint is identified by (stream_id, version); the worker upserts
-- it, and under JetStream redelivery the same (stream_id, version) may be
-- written twice. This constraint makes that upsert idempotent and makes the
-- "latest checkpoint by version" resume lookup unambiguous.
ALTER TABLE snapshots
    ADD CONSTRAINT uq_snapshots_stream_version UNIQUE (stream_id, version);
