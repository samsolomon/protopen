alter table deploys add column if not exists git_commit_hash text;
alter table deploys add column if not exists git_branch text;
alter table deploys add column if not exists git_commit_message text;
alter table deploys add column if not exists git_dirty boolean;
alter table deploys add column if not exists git_author text;
alter table deploys add column if not exists git_remote_url text;
