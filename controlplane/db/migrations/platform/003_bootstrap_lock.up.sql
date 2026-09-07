-- bootstrap_lock — atomic election of the very first user as admin.
--
-- The auth callback promotes the first-ever user to admin (endpoints.yaml,
-- auth.callback branch "bootstrap"). Deciding that on SELECT count(*) = 0 is a
-- race: two simultaneous first logins can both read 0 and both become admin.
--
-- The primary key is a BOOLEAN pinned TRUE by CHECK, so the table can hold at
-- most ONE row ever. The first INSERT wins; every later one gets a unique
-- violation (SQLSTATE 23505), which the caller reads as "someone else
-- bootstrapped" and falls through to the normal pending-user path. The row is
-- never deleted — bootstrap is a once-per-installation event.
CREATE TABLE bootstrap_lock (
    id         BOOLEAN     PRIMARY KEY DEFAULT TRUE,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bootstrap_lock_id_must_be_true CHECK (id IS TRUE)
);
