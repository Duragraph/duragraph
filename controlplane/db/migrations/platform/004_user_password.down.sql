-- Reverses 004_user_password.
--
-- Password-only users are deleted: without password_hash they would be
-- rows with no way to authenticate at all. That is destructive and
-- deliberate — a password-only account cannot be represented in the
-- pre-004 schema, so there is no lossless way down.
--
-- The oauth columns are left nullable because 001 declared them that
-- way; this migration never changed them.

DROP INDEX IF EXISTS idx_users_lower_email;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_at_least_one_auth_method;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_auth_method_check;

DELETE FROM users WHERE oauth_provider IS NULL;

ALTER TABLE users
    DROP COLUMN IF EXISTS password_hash,
    DROP COLUMN IF EXISTS auth_method;
