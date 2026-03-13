create table if not exists wallet_accounts (
  user_id text primary key,
  balance_fen bigint not null default 0,
  currency text not null default 'CNY',
  updated_at timestamptz not null default now()
);

create table if not exists wallet_transactions (
  id text primary key,
  user_id text not null references wallet_accounts(user_id),
  kind text not null,
  amount_fen bigint not null,
  description text not null default '',
  reference_type text not null default '',
  reference_id text not null default '',
  pricing_version text not null default '',
  created_at timestamptz not null default now()
);

create table if not exists recharge_orders (
  id text primary key,
  user_id text not null references wallet_accounts(user_id),
  amount_fen bigint not null,
  refunded_fen bigint not null default 0,
  channel text not null,
  provider text not null default 'manual',
  status text not null,
  pay_url text not null default '',
  external_order_id text not null default '',
  provider_status text not null default '',
  pricing_version text not null default '',
  agreement_versions text[] not null default '{}',
  paid_at timestamptz,
  last_checked_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists admin_users (
  email text primary key,
  user_id text,
  active boolean not null default true,
  created_at timestamptz not null default now()
);

create table if not exists user_agreements (
  user_id text not null,
  agreement_key text not null,
  version text not null,
  accepted_at timestamptz not null default now(),
  client_version text not null default '',
  remote_addr text not null default '',
  device_summary text not null default '',
  content_checksum text not null default '',
  primary key (user_id, agreement_key, version)
);

create table if not exists chat_usage_records (
  id text primary key,
  user_id text not null,
  model_id text not null,
  pricing_version text not null default '',
  prompt_tokens integer not null default 0,
  completion_tokens integer not null default 0,
  charged_fen bigint not null default 0,
  fallback_applied boolean not null default false,
  request_kind text not null default '',
  agreement_versions text[] not null default '{}',
  created_at timestamptz not null default now()
);

create table if not exists admin_audit_logs (
  id text primary key,
  actor_user_id text not null default '',
  actor_email text not null default '',
  action text not null,
  target_type text not null default '',
  target_id text not null default '',
  risk_level text not null default '',
  detail text not null default '',
  created_at timestamptz not null default now()
);

create table if not exists refund_requests (
  id text primary key,
  user_id text not null references wallet_accounts(user_id),
  order_id text not null references recharge_orders(id),
  amount_fen bigint not null,
  reason text not null default '',
  status text not null,
  review_note text not null default '',
  reviewed_by text not null default '',
  refund_provider text not null default '',
  external_refund_id text not null default '',
  external_status text not null default '',
  failure_reason text not null default '',
  settled_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists infringement_reports (
  id text primary key,
  user_id text not null,
  subject text not null,
  description text not null,
  evidence_urls text[] not null default '{}',
  status text not null,
  resolution text not null default '',
  reviewed_by text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists data_retention_policies (
  data_domain text primary key,
  retention_days integer not null,
  purge_mode text not null default '',
  description text not null default '',
  enabled boolean not null default true,
  updated_at timestamptz not null default now()
);

create table if not exists system_notices (
  id text primary key,
  title text not null,
  body text not null,
  severity text not null default '',
  enabled boolean not null default true,
  updated_at timestamptz not null default now()
);

create table if not exists risk_rules (
  key text primary key,
  name text not null,
  description text not null default '',
  enabled boolean not null default true,
  updated_at timestamptz not null default now()
);
