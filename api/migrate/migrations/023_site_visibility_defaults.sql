-- SPDX-License-Identifier: AGPL-3.0-only

-- Tracks the moment a site most recently became public. NULL while is_public
-- is false. Reset to now() on every false→true transition; nulled on the
-- reverse transition. Used by the auto-private sweeper to find sites older
-- than the admin-configured threshold.
alter table sites add column made_public_at timestamptz;

-- Backfill existing public sites so the sweeper has a clock to measure
-- against. The user-chosen semantics is "no grandfathering": when an admin
-- first enables auto-private, every currently-public site becomes eligible
-- (subject to the batch cap in the sweeper).
update sites set made_public_at = created_at
  where is_public = true and deleted_at is null;

-- Partial index narrowed to the sweeper's WHERE clause so it touches only
-- the rows it can ever care about.
create index sites_made_public_at_idx on sites (made_public_at)
  where is_public = true and deleted_at is null and made_public_at is not null;

-- Allow system-actor rows in audit_log so the auto-private sweeper can log
-- reverts without inventing a phantom user. The actor_email column carries
-- the sentinel ("system:auto-private") for these rows.
alter table audit_log alter column actor_user_id drop not null;
