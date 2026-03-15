alter table admin_users
  add column if not exists role text not null default 'super_admin',
  add column if not exists capabilities text[] not null default '{}',
  add column if not exists updated_at timestamptz not null default now();

update admin_users
set role = case when coalesce(nullif(role, ''), '') = '' then 'super_admin' else role end,
    capabilities = case
      when coalesce(array_length(capabilities, 1), 0) = 0 then array[
        'agreements.read',
        'agreements.write',
        'audit.read',
        'dashboard.read',
        'infringement.read',
        'infringement.review',
        'models.read',
        'models.write',
        'notices.read',
        'notices.write',
        'operators.read',
        'operators.write',
        'orders.read',
        'orders.write',
        'pricing.read',
        'pricing.write',
        'refunds.read',
        'refunds.review',
        'retention.read',
        'retention.write',
        'risk.read',
        'risk.write',
        'routes.read',
        'routes.write',
        'runtime.read',
        'runtime.write',
        'users.read',
        'users.write',
        'wallet.read',
        'wallet.write'
      ]::text[]
      else capabilities
    end,
    updated_at = now();
