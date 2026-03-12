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
  created_at timestamptz not null default now()
);

create table if not exists recharge_orders (
  id text primary key,
  user_id text not null references wallet_accounts(user_id),
  amount_fen bigint not null,
  channel text not null,
  provider text not null default 'manual',
  status text not null,
  pay_url text not null default '',
  external_order_id text not null default '',
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
  primary key (user_id, agreement_key)
);
