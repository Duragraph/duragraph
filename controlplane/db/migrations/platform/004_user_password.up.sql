-- 004_user_password.up.sql: email + password authentication.
--
-- 001_platform created users for OAuth only: oauth_provider and oauth_id
-- are NOT NULL, and there is nowhere to put a password. Password auth
-- needs both relaxed and a hash column.
--
-- Decisions:
--
-- 1. oauth_provider / oauth_id are ALREADY nullable in 001, and the
--    unique index there is restricted to `WHERE oauth_provider IS NOT
--    NULL`, so password-only rows neither violate it nor collide on it.
--    No ALTER is needed for that — the equivalent legacy migration
--    (internal/.../005_user_password) does drop NOT NULL, because the
--    legacy schema declared it; this one does not.
--
-- 2. password_hash is NULLABLE — OAuth-only users have none. It holds a
--    bcrypt digest, never a plaintext or reversible encoding.
--
-- 3. auth_method defaults to 'oauth' so rows written before this
--    migration keep their meaning without a backfill.
--
-- 4. A row must carry at least one way to authenticate. Without this a
--    bug could insert a user with neither an OAuth identity nor a
--    password — an account nobody can ever sign in to, and which no
--    login path would report as broken.
--
--    NOTE: this CHECK is validated against existing rows, so the
--    migration fails if any user already has neither. Both code paths
--    that insert users set one or the other (auth.go sets oauth_provider,
--    auth_password.go sets password_hash), so such a row can only come
--    from manual insertion — and it is an account that cannot be used.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS password_hash TEXT NULL,
    ADD COLUMN IF NOT EXISTS auth_method   VARCHAR(20) NOT NULL DEFAULT 'oauth';

ALTER TABLE users
    ADD CONSTRAINT users_auth_method_check
    CHECK (auth_method IN ('oauth', 'password'));

ALTER TABLE users
    ADD CONSTRAINT users_at_least_one_auth_method
    CHECK (oauth_provider IS NOT NULL OR password_hash IS NOT NULL);

-- Login matches on LOWER(email) so addresses differing only in case
-- resolve to one account. The UNIQUE constraint from 001 is
-- case-SENSITIVE, so this index is both the lookup path and the thing
-- that makes case-insensitive matching affordable.
--
-- Note this index is not UNIQUE: making it so would fail on any existing
-- pair of rows differing only by case. Registration guards against new
-- case-duplicates instead (see AuthRegister).
CREATE INDEX IF NOT EXISTS idx_users_lower_email ON users (LOWER(email));
