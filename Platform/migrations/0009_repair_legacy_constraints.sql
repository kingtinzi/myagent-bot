alter table user_agreements
  add column if not exists version text;

update user_agreements
set version = 'v1'
where coalesce(trim(version), '') = '';

alter table user_agreements
  alter column version set default 'v1',
  alter column version set not null;

do $$
begin
  if exists (
    select 1
    from pg_constraint
    where conrelid = 'public.user_agreements'::regclass
      and contype = 'p'
      and pg_get_constraintdef(oid) <> 'PRIMARY KEY (user_id, agreement_key, version)'
  ) then
    alter table public.user_agreements
      drop constraint user_agreements_pkey;
  end if;

  if not exists (
    select 1
    from pg_constraint
    where conrelid = 'public.user_agreements'::regclass
      and contype = 'p'
      and pg_get_constraintdef(oid) = 'PRIMARY KEY (user_id, agreement_key, version)'
  ) then
    alter table public.user_agreements
      add constraint user_agreements_pkey primary key (user_id, agreement_key, version);
  end if;
end $$;

drop index if exists wallet_transactions_reference_unique_idx;

create unique index if not exists wallet_transactions_reference_unique_idx
  on wallet_transactions (reference_type, reference_id)
  where reference_type <> '' and reference_id <> '';
