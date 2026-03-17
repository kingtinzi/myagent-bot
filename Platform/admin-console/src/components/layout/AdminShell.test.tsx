import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';

import { AdminShell } from './AdminShell';
import type { AdminNavigationGroup } from './AdminSidebar';
import { PageHeader } from './PageHeader';

const navigationGroups: AdminNavigationGroup[] = [
  {
    id: 'overview',
    label: '总览',
    items: [
      { id: 'dashboard', to: '/dashboard', label: '仪表盘', description: '关键指标、活跃度与风险总览' },
      { id: 'users', to: '/users', label: '用户', description: '账号、钱包与协议签署记录' },
    ],
  },
  {
    id: 'finance',
    label: '财务与目录',
    items: [{ id: 'wallet', to: '/wallet', label: '钱包', description: '手动充值、调账与账本流水' }],
  },
  {
    id: 'governance',
    label: '控制与审核',
    items: [{ id: 'audits', to: '/audits', label: '审计', description: '敏感操作轨迹与导出审计' }],
  },
];

describe('AdminShell', () => {
  it('renders grouped navigation, topbar chrome and page header actions', () => {
    render(
      <MemoryRouter initialEntries={['/users']}>
        <AdminShell
          navigationGroups={navigationGroups}
          onRefresh={() => undefined}
          onSignOut={() => undefined}
          operator={{
            displayName: '俊夫',
            meta: '超级管理员 · 平台治理 / 财务 / 内容审核',
            roleLabel: '超级管理员',
            roleTone: 'success',
          }}
          toast={{ title: '后台重构进行中', message: '新版后台壳层已建立。', tone: 'info' }}
        >
          <PageHeader
            description="统一管理用户身份、注册协议签署、余额概览与高级行为轨迹。"
            title="用户与账号"
            actions={
              <button className="button button--primary" type="button">
                新增管理员
              </button>
            }
          />
        </AdminShell>
      </MemoryRouter>,
    );

    expect(screen.getByText('总览')).toBeInTheDocument();
    expect(screen.getByText('财务与目录')).toBeInTheDocument();
    expect(screen.getByText('控制与审核')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /仪表盘/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '展开导航' })).toBeInTheDocument();
    expect(screen.getByText('俊夫')).toBeInTheDocument();
    expect(screen.getByText('超级管理员')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '刷新后台' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '退出登录' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '用户与账号' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '新增管理员' })).toBeInTheDocument();
  });
});
