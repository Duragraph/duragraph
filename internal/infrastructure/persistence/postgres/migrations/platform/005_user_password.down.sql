-- 005_user_password.down.sql: rollback the password-auth schema additions.
--
-- Drops password_hash + auth_method, restores the OAuth-only CHECK
-- constraints. WARNING: any password-only user rows would violate the
-- restored NOT NULL on oauth_provider; the down migration first deletes
-- those rows. This is intentional — rollback is a recovery path, not
-- a soft-toggle.

DELETE FROM platform.users WHERE oauth_provider IS NULL;

ALTER TABLE platform.users DROP CONSTRAINT IF EXISTS users_at_least_one_auth_method;
ALTER TABLE platform.users DROP CONSTRAINT IF EXISTS users_auth_method_check;
ALTER TABLE platform.users DROP CONSTRAINT IF EXISTS users_oauth_provider_check;

ALTER TABLE platform.users
    ADD CONSTRAINT users_oauth_provider_check
    CHECK (oauth_provider IN ('google', 'github'));

ALTER TABLE platform.users
    ALTER COLUMN oauth_provider SET NOT NULL,
    ALTER COLUMN oauth_id       SET NOT NULL;

ALTER TABLE platform.users
    DROP COLUMN IF EXISTS password_hash,
    DROP COLUMN IF EXISTS auth_method;

DROP INDEX IF EXISTS platform.idx_users_lower_email;
