alter table admin_users
  add column if not exists managed_by_seed boolean not null default false;

update admin_users
set managed_by_seed = true
where lower(coalesce(role, '')) = 'super_admin';
