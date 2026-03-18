import type { AdminNavigationGroup, AdminNavigationItem } from '../components/layout/AdminSidebar';
import type { AdminModuleKey } from '../hooks/useCapabilities';

type AdminNavigationDefinitionItem = AdminNavigationItem & {
  moduleKey: AdminModuleKey;
};

type AdminNavigationDefinitionGroup = {
  id: string;
  label: string;
  items: AdminNavigationDefinitionItem[];
};

export const adminNavigationDefinitions: AdminNavigationDefinitionGroup[] = [
  {
    id: 'overview',
    label: '总览',
    items: [
      { id: 'dashboard', moduleKey: 'dashboard', to: '/dashboard', label: '仪表盘', description: '关键指标、活跃度与风险总览' },
      { id: 'users', moduleKey: 'users', to: '/users', label: '用户', description: '账号、钱包与协议签署记录' },
      { id: 'operators', moduleKey: 'operators', to: '/operators', label: '管理员', description: '角色、权限与后台值班配置' },
    ],
  },
  {
    id: 'finance',
    label: '财务与目录',
    items: [
      { id: 'orders', moduleKey: 'orders', to: '/orders', label: '订单', description: '充值订单、支付链路与对账' },
      { id: 'wallet', moduleKey: 'wallet', to: '/wallet', label: '钱包', description: '手动充值、调账与账本流水' },
      {
        id: 'catalog',
        moduleKey: 'catalog',
        to: '/catalog',
        label: '目录',
        description: '模型、路由、价格与协议版本',
        badge: '核心',
        badgeTone: 'info',
      },
    ],
  },
  {
    id: 'governance',
    label: '控制与审核',
    items: [
      { id: 'audits', moduleKey: 'audits', to: '/audits', label: '审计', description: '敏感操作轨迹与导出审计' },
      { id: 'refunds', moduleKey: 'refunds', to: '/refunds', label: '退款', description: '退款审核、驳回与结算' },
      { id: 'infringement', moduleKey: 'infringement', to: '/infringement', label: '侵权', description: '案件处理、证据与结论' },
      {
        id: 'governance',
        moduleKey: 'governance',
        to: '/governance',
        label: '治理',
        description: '公告、风控规则与保留策略',
        badge: '策略',
        badgeTone: 'warning',
      },
    ],
  },
];

export function buildNavigationGroups(canAccessModule: (moduleKey: AdminModuleKey) => boolean): AdminNavigationGroup[] {
  return adminNavigationDefinitions
    .map(group => ({
      id: group.id,
      label: group.label,
      items: group.items
        .filter(item => canAccessModule(item.moduleKey))
        .map(({ moduleKey: _moduleKey, ...item }) => item),
    }))
    .filter(group => group.items.length > 0);
}

export function firstAccessiblePath(canAccessModule: (moduleKey: AdminModuleKey) => boolean) {
  for (const group of adminNavigationDefinitions) {
    for (const item of group.items) {
      if (canAccessModule(item.moduleKey)) {
        return item.to;
      }
    }
  }

  return '';
}
