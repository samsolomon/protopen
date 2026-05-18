-- SPDX-License-Identifier: AGPL-3.0-only

alter table sites
    add column created_by text references users(id) on delete set null;

-- Backfill: only personal orgs with exactly one member are unambiguously
-- authored by that member. Any org with more than one member (including
-- mis-flagged is_personal=true orgs used as team orgs in dev seed data) is
-- skipped, leaving created_by NULL. The safe failure mode is admin-only
-- mutation rather than false attribution.
update sites s
set created_by = m.user_id
from organizations o
join org_members m on m.org_id = o.id
where s.org_id = o.id
  and o.is_personal = true
  and s.created_by is null
  and (select count(*) from org_members where org_id = o.id) = 1;

create index sites_org_created_by_idx
    on sites (org_id, created_by)
    where deleted_at is null;
