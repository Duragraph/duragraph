-- 004_bootstrap_lock.down.sql: Drop the bootstrap_lock table.
--
-- Reverse of 004_bootstrap_lock.up.sql. Reapplying the up migration after
-- a down + up cycle resets the lock and would allow a second
-- bootstrap-first-user election; this is intentional and matches the
-- semantics of `task db:reset` (a destructive operator-driven reset).

DROP TABLE IF EXISTS platform.bootstrap_lock;
