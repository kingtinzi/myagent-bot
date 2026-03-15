alter table wallet_transactions
  add column if not exists reference_type text,
  add column if not exists reference_id text,
  add column if not exists pricing_version text;

update wallet_transactions
set
  reference_type = coalesce(reference_type, ''),
  reference_id = coalesce(reference_id, ''),
  pricing_version = coalesce(pricing_version, '')
where
  reference_type is null
  or reference_id is null
  or pricing_version is null;

alter table wallet_transactions
  alter column reference_type set default '',
  alter column reference_id set default '',
  alter column pricing_version set default '';

alter table wallet_transactions
  alter column reference_type set not null,
  alter column reference_id set not null,
  alter column pricing_version set not null;

alter table recharge_orders
  add column if not exists refunded_fen bigint,
  add column if not exists provider_status text,
  add column if not exists pricing_version text,
  add column if not exists agreement_versions text[],
  add column if not exists paid_at timestamptz,
  add column if not exists last_checked_at timestamptz;

update recharge_orders
set
  refunded_fen = coalesce(refunded_fen, 0),
  provider_status = coalesce(provider_status, ''),
  pricing_version = coalesce(pricing_version, ''),
  agreement_versions = coalesce(agreement_versions, '{}'::text[])
where
  refunded_fen is null
  or provider_status is null
  or pricing_version is null
  or agreement_versions is null;

alter table recharge_orders
  alter column refunded_fen set default 0,
  alter column provider_status set default '',
  alter column pricing_version set default '',
  alter column agreement_versions set default '{}'::text[];

alter table recharge_orders
  alter column refunded_fen set not null,
  alter column provider_status set not null,
  alter column pricing_version set not null,
  alter column agreement_versions set not null;

alter table user_agreements
  add column if not exists client_version text,
  add column if not exists remote_addr text,
  add column if not exists device_summary text,
  add column if not exists content_checksum text;

update user_agreements
set
  client_version = coalesce(client_version, ''),
  remote_addr = coalesce(remote_addr, ''),
  device_summary = coalesce(device_summary, ''),
  content_checksum = coalesce(content_checksum, '')
where
  client_version is null
  or remote_addr is null
  or device_summary is null
  or content_checksum is null;

alter table user_agreements
  alter column client_version set default '',
  alter column remote_addr set default '',
  alter column device_summary set default '',
  alter column content_checksum set default '';

alter table user_agreements
  alter column client_version set not null,
  alter column remote_addr set not null,
  alter column device_summary set not null,
  alter column content_checksum set not null;
