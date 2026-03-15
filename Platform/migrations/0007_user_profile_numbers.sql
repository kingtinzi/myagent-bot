create sequence if not exists user_profiles_user_no_seq
  as bigint
  start with 1
  increment by 1;

insert into user_profiles (user_id, email, created_at, updated_at, last_seen_at)
select src.user_id, '', now(), now(), now()
from (
  select user_id from wallet_accounts
  union
  select user_id from recharge_orders
  union
  select user_id from refund_requests
  union
  select user_id from user_agreements
  union
  select user_id from chat_usage_records
  union
  select user_id from infringement_reports
) src
where coalesce(src.user_id, '') <> ''
on conflict (user_id) do nothing;

alter table user_profiles
  add column if not exists user_no bigint;

alter table user_profiles
  alter column user_no set default nextval('user_profiles_user_no_seq');

update user_profiles
set user_no = nextval('user_profiles_user_no_seq')
where user_no is null;

select setval(
  'user_profiles_user_no_seq',
  coalesce((select max(user_no) from user_profiles), 0) + 1,
  false
);

alter table user_profiles
  alter column user_no set not null;

create unique index if not exists idx_user_profiles_user_no
  on user_profiles (user_no);
