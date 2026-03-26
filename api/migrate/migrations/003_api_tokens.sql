create table if not exists api_tokens (
  id text primary key,
  user_id text not null references users(id) on delete cascade,
  token_hash text not null unique,
  name text not null,
  created_at timestamptz not null default now()
);

create index if not exists api_tokens_user_id_idx on api_tokens (user_id);
