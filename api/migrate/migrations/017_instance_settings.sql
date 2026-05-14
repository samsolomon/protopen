-- SPDX-License-Identifier: AGPL-3.0-only

create table instance_settings (
    key text primary key,
    value text not null,
    updated_at timestamptz not null default now(),
    updated_by text
);
