create table if not exists device_codes (
  id          text primary key,
  code        text not null unique,
  user_id     text references users(id),
  token       text,
  status      text not null default 'pending',
  created_at  timestamptz not null default now(),
  expires_at  timestamptz not null
);
