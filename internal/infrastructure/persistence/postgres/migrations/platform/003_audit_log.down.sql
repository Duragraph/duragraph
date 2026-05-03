-- 003_audit_log.down.sql: Reverse the audit_log table.
-- Idempotent: safe to run when the table doesn't exist.

DROP TABLE IF EXISTS platform.audit_log CASCADE;
