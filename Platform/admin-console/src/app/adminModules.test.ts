import { describe, expect, it } from 'vitest';

import { buildNavigationGroups, firstAccessiblePath } from './adminModules';
import type { AdminModuleKey } from '../hooks/useCapabilities';

describe('adminModules navigation helpers', () => {
  it('builds navigation groups from accessible modules only', () => {
    const accessibleModules = new Set<AdminModuleKey>(['dashboard', 'orders', 'wallet']);
    const canAccessModule = (moduleKey: AdminModuleKey) => accessibleModules.has(moduleKey);

    expect(buildNavigationGroups(canAccessModule)).toEqual([
      {
        id: 'overview',
        label: '总览',
        items: [{ id: 'dashboard', to: '/dashboard', label: '仪表盘', description: '关键指标、活跃度与风险总览' }],
      },
      {
        id: 'finance',
        label: '财务与目录',
        items: [
          { id: 'orders', to: '/orders', label: '订单', description: '充值订单、支付链路与对账' },
          { id: 'wallet', to: '/wallet', label: '钱包', description: '手动充值、调账与账本流水' },
        ],
      },
    ]);
  });

  it('returns the first accessible path in navigation order', () => {
    const accessibleModules = new Set<AdminModuleKey>(['wallet', 'governance']);
    const canAccessModule = (moduleKey: AdminModuleKey) => accessibleModules.has(moduleKey);

    expect(firstAccessiblePath(canAccessModule)).toBe('/wallet');
  });

  it('returns an empty path when no module is accessible', () => {
    expect(firstAccessiblePath(() => false)).toBe('');
    expect(buildNavigationGroups(() => false)).toEqual([]);
  });
});
