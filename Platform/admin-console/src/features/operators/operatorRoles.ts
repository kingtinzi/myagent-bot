import type { StatusTone } from '../../components/display/StatusBadge';

export type AdminRoleKey = 'super_admin' | 'read_only' | 'finance' | 'governance' | 'operations';

export type AdminRoleDefinition = {
  value: AdminRoleKey;
  label: string;
  tone: StatusTone;
  summary: string;
  dangerNote?: string;
  capabilities: string[];
};

export const adminRoleDefinitions: AdminRoleDefinition[] = [
  {
    value: 'super_admin',
    label: '超级管理员',
    tone: 'success',
    summary: '拥有全部后台模块与敏感操作能力，适用于平台 owner 或最终审批人。',
    dangerNote: '超级管理员变更会授予所有敏感后台模块的访问权限。',
    capabilities: [
      'dashboard.read',
      'users.read',
      'users.write',
      'operators.read',
      'operators.write',
      'orders.read',
      'orders.write',
      'wallet.read',
      'wallet.write',
      'models.read',
      'models.write',
      'routes.read',
      'routes.write',
      'pricing.read',
      'pricing.write',
      'agreements.read',
      'agreements.write',
      'usage.read',
      'audit.read',
      'refunds.read',
      'refunds.review',
      'infringement.read',
      'infringement.review',
      'notices.read',
      'notices.write',
      'risk.read',
      'risk.write',
      'retention.read',
      'retention.write',
      'runtime.read',
      'runtime.write',
    ],
  },
  {
    value: 'read_only',
    label: '只读管理员',
    tone: 'neutral',
    summary: '只读巡检与排障角色，只能查看后台数据，不能执行写操作。',
    capabilities: [
      'dashboard.read',
      'users.read',
      'operators.read',
      'orders.read',
      'wallet.read',
      'models.read',
      'pricing.read',
      'agreements.read',
      'usage.read',
      'audit.read',
      'refunds.read',
      'infringement.read',
      'notices.read',
      'risk.read',
      'retention.read',
    ],
  },
  {
    value: 'finance',
    label: '财务管理员',
    tone: 'info',
    summary: '面向充值、钱包、退款场景，负责对账、人工入账与退款审核。',
    capabilities: ['dashboard.read', 'users.read', 'orders.read', 'orders.write', 'wallet.read', 'wallet.write', 'audit.read', 'refunds.read', 'refunds.review'],
  },
  {
    value: 'governance',
    label: '治理管理员',
    tone: 'warning',
    summary: '负责协议、公告、风控与侵权治理，强调审计与风险留痕。',
    capabilities: [
      'dashboard.read',
      'users.read',
      'agreements.read',
      'agreements.write',
      'audit.read',
      'refunds.read',
      'refunds.review',
      'infringement.read',
      'infringement.review',
      'notices.read',
      'notices.write',
      'risk.read',
      'risk.write',
      'retention.read',
      'retention.write',
    ],
  },
  {
    value: 'operations',
    label: '运营管理员',
    tone: 'info',
    summary: '负责模型目录、路由、价格、协议与运行时配置的日常维护。',
    capabilities: [
      'dashboard.read',
      'users.read',
      'wallet.read',
      'wallet.write',
      'models.read',
      'models.write',
      'routes.read',
      'routes.write',
      'pricing.read',
      'pricing.write',
      'agreements.read',
      'agreements.write',
      'usage.read',
      'audit.read',
      'runtime.read',
      'runtime.write',
    ],
  },
];

export function getAdminRoleDefinition(role: string | undefined | null) {
  return adminRoleDefinitions.find(item => item.value === role) ?? adminRoleDefinitions[0];
}
