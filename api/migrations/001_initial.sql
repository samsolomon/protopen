create table if not exists users (
  id text primary key,
  email text not null unique,
  auth_ref text not null,
  username text not null unique,
  name text not null,
  created_at timestamptz not null default now()
);

create table if not exists projects (
  id text primary key,
  user_id text not null references users(id),
  slug text not null,
  name text not null,
  current_deploy_id text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);

create unique index if not exists projects_user_slug_active_idx
  on projects (user_id, slug)
  where deleted_at is null;

create table if not exists deploys (
  id text primary key,
  project_id text not null references projects(id),
  status text not null,
  size_bytes bigint not null,
  file_count integer not null,
  storage_prefix text not null,
  created_at timestamptz not null default now()
);

alter table projects
  add constraint projects_current_deploy_id_fkey
  foreign key (current_deploy_id) references deploys(id);
