-- 004_bootstrap_lock.up.sql: Single-row guard for the OAuth callback
-- bootstrap-first-user branch.
--
-- Per duragraph-spec/auth/oauth.yml § callback_flow.bootstrap_first_user.atomicity,
-- the very-first-user election is race-prone: two concurrent OAuth callbacks
-- can both observe `COUNT(*) = 0` on platform.users and both attempt to
-- self-elevate to admin. The spec offers two atomic patterns; we implement
-- the bootstrap-lock variant (option (b)) because the existing UserRepository
-- and TenantRepository do not currently expose a `WithTx` variant suitable
-- for wrapping the multi-step bootstrap branch in a single SERIALIZABLE
-- transaction (option (a) would require refactoring those repositories,
-- which is out of scope for this PR).
--
-- Semantics:
--   * Exactly one row may ever exist (PRIMARY KEY on a boolean column with
--     a CHECK pinning it to true).
--   * The OAuth callback handler attempts `INSERT INTO
--     platform.bootstrap_lock (id) VALUES (true)` inside the bootstrap
--     branch. The first caller succeeds and proceeds to register the
--     auto-admin user + tenant. Concurrent callers receive
--     unique_violation (SQLSTATE 23505) and fall through to the new_user
--     branch.
--   * `claimed_at` is informational; useful for forensic queries
--     ("when did this deployment first sign in?") but not part of the
--     atomicity guarantee.
--   * The row is NEVER deleted in normal operation. Once the platform
--     has a first user, the lock has done its job.

CREATE TABLE IF NOT EXISTS platform.bootstrap_lock (
    id         BOOLEAN     PRIMARY KEY DEFAULT TRUE,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT bootstrap_lock_id_must_be_true CHECK (id IS TRUE)
);
