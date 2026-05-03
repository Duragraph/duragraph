-- 002_init_tenants.down.sql: Reverse the tenants table.
-- Idempotent: safe to run when the table doesn't exist.

DROP TABLE IF EXISTS platform.tenants CASCADE;
