-- 003_audit_log.up.sql: Platform audit log (append-only).
--
-- Populated by the platform-audit-users / platform-audit-tenants NATS
-- consumers (see duragraph-spec/async/asyncapi.yml — operations
-- subscribePlatformAuditUsers / subscribePlatformAuditTenants), which
-- project user.* and tenant.* events into this table for replay/audit.
--
-- DESIGN NOTES:
-- - Append-only: NO update trigger. Once a row is inserted it must
--   never change. Operators wanting to "correct" an audit record should
--   insert a new compensating event instead.
-- - NO foreign key to platform.users. Events outlive their actors —
--   even if a user is later deleted (which is currently RESTRICTed by
--   the tenants FK, but a future hard-delete path may exist), the audit
--   trail must remain intact. A cascading FK would corrupt history.
-- - `aggregate_id` references either a user_id or a tenant_id depending
--   on aggregate_type; not constrained at the schema level because the
--   FK rule above applies to both.
-- - `occurred_at` carries the event's own timestamp (when the original
--   domain event happened); `recorded_at` is the audit-row insert time.
--   Distinguishing the two matters for late-arriving events / replay.

CREATE TABLE IF NOT EXISTS platform.audit_log (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type     VARCHAR(100) NOT NULL,
    aggregate_type VARCHAR(20)  NOT NULL,
    aggregate_id   UUID         NOT NULL,
    actor_user_id  UUID         NULL,
    payload        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    reason         TEXT         NULL,
    ip_address     INET         NULL,
    user_agent     TEXT         NULL,
    occurred_at    TIMESTAMPTZ  NOT NULL,
    recorded_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_aggregate
    ON platform.audit_log (aggregate_type, aggregate_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_event_type
    ON platform.audit_log (event_type);
CREATE INDEX IF NOT EXISTS idx_audit_log_occurred_at
    ON platform.audit_log (occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor_user_id
    ON platform.audit_log (actor_user_id);
