create table if not exists user_profiles (
  user_id text primary key,
  email text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now()
);
