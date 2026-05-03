-- 001_init_users.down.sql: Reverse the users table + helper function.
-- Idempotent: safe to run when objects don't exist.

DROP TABLE IF EXISTS platform.users CASCADE;
DROP FUNCTION IF EXISTS platform.update_updated_at_column() CASCADE;
