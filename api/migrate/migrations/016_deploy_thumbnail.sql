-- SPDX-License-Identifier: AGPL-3.0-only

alter table deploys add column thumbnail_path text;

-- Supports the backstop loop's "recent deploys missing a thumbnail" scan.
create index deploys_thumbnail_pending_idx
    on deploys (created_at desc)
    where thumbnail_path is null;
