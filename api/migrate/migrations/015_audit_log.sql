-- SPDX-License-Identifier: AGPL-3.0-only

create table audit_log (
    id text primary key,
    actor_user_id text not null references users(id),
    actor_email text not null,
    action text not null,
    target_type text not null,
    target_id text not null,
    metadata jsonb,
    created_at timestamptz not null default now()
);

create index audit_log_created_at_idx on audit_log (created_at desc);
create index audit_log_actor_idx on audit_log (actor_user_id, created_at desc);
