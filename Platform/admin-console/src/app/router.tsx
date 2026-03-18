import type { ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Navigate, Outlet, createHashRouter, useLocation } from 'react-router-dom';

import { buildNavigationGroups, firstAccessiblePath } from './adminModules';
import { EmptyState } from '../components/display/EmptyState';
import { type StatusTone } from '../components/display/StatusBadge';
import { AdminShell, type AdminShellOperator } from '../components/layout/AdminShell';
import { useAdminSession } from '../hooks/useAdminSession';
import { useCapabilities, type AdminModuleKey } from '../hooks/useCapabilities';
import type { AdminSession } from '../services/contracts';
import { DashboardPage } from '../pages/dashboard/DashboardPage';
import { AuditsPage } from '../pages/audits/AuditsPage';
import { CatalogPage } from '../pages/catalog/CatalogPage';
import { GovernancePage } from '../pages/governance/GovernancePage';
import { InfringementPage } from '../pages/infringement/InfringementPage';
import { OrdersPage } from '../pages/orders/OrdersPage';
import { OperatorsPage } from '../pages/operators/OperatorsPage';
import { RefundsPage } from '../pages/refunds/RefundsPage';
import { UsersPage } from '../pages/users/UsersPage';
import { WalletPage } from '../pages/wallet/WalletPage';

function QueryProviderProbe() {
  const queryClient = useQueryClient();

  return (
    <output hidden data-testid="query-provider-ready">
      {queryClient ? 'ready' : 'missing'}
    </output>
  );
}

function roleLabel(role: string | undefined) {
  switch ((role ?? '').trim()) {
    case 'super_admin':
      return '超级管理员';
    case 'finance':
      return '财务管理员';
    case 'governance':
      return '治理管理员';
    case 'operations':
      return '运营管理员';
    case 'read_only':
      return '只读管理员';
    default:
      return '管理员';
  }
}

function roleTone(role: string | undefined): StatusTone {
  switch ((role ?? '').trim()) {
    case 'super_admin':
      return 'success';
    case 'finance':
      return 'info';
    case 'governance':
      return 'warning';
    case 'operations':
      return 'info';
    case 'read_only':
      return 'neutral';
    default:
      return 'info';
  }
}

function roleMeta(role: string | undefined, capabilityCount: number) {
  switch ((role ?? '').trim()) {
    case 'super_admin':
      return '全部后台模块与敏感操作';
    case 'finance':
      return '订单、钱包与退款处理';
    case 'governance':
      return '协议、公告与风控治理';
    case 'operations':
      return '模型目录、路由与价格配置';
    case 'read_only':
      return capabilityCount > 0 ? `只读巡检 · 已启用 ${capabilityCount} 项查看能力` : '只读巡检';
    default:
      return capabilityCount > 0 ? `已启用 ${capabilityCount} 项后台能力` : '等待分配后台能力';
  }
}

function buildOperatorSummary(session: AdminSession | null): AdminShellOperator {
  const email = session?.user.email?.trim() || session?.operator.email?.trim() || '管理员';
  const capabilityCount = session?.operator.capabilities.length ?? 0;

  return {
    displayName: email,
    meta: roleMeta(session?.operator.role, capabilityCount),
    roleLabel: roleLabel(session?.operator.role),
    roleTone: roleTone(session?.operator.role),
  };
}

function NoAccessibleModuleState() {
  return (
    <section className="page-stack">
      <EmptyState
        eyebrow="权限不足"
        title="当前管理员尚未分配可访问模块"
        description="请联系超级管理员为该账号配置后台角色与 capability，然后重新登录后台。"
      />
    </section>
  );
}

function ProtectedModuleRoute({
  moduleKey,
  children,
}: {
  moduleKey: AdminModuleKey;
  children: ReactNode;
}) {
  const location = useLocation();
  const capabilities = useCapabilities();
  const fallbackPath = firstAccessiblePath(capabilities.canAccessModule);

  if (capabilities.canAccessModule(moduleKey)) {
    return <>{children}</>;
  }

  if (!fallbackPath) {
    return <NoAccessibleModuleState />;
  }

  if (fallbackPath === location.pathname) {
    return <NoAccessibleModuleState />;
  }

  return <Navigate replace state={{ deniedFrom: location.pathname }} to={fallbackPath} />;
}

function HomeRedirect() {
  const capabilities = useCapabilities();
  const destination = firstAccessiblePath(capabilities.canAccessModule);

  if (!destination) {
    return <NoAccessibleModuleState />;
  }

  return <Navigate replace to={destination} />;
}

function AdminLayout() {
  const queryClient = useQueryClient();
  const { session, logout } = useAdminSession();
  const capabilities = useCapabilities(session?.operator);
  const navigationGroups = buildNavigationGroups(capabilities.canAccessModule);

  return (
    <AdminShell
      navigationGroups={navigationGroups}
      onRefresh={() => {
        void queryClient.invalidateQueries({ queryKey: ['admin'] });
      }}
      onSignOut={() => {
        void logout();
      }}
      operator={buildOperatorSummary(session)}
    >
      <QueryProviderProbe />
      <Outlet />
    </AdminShell>
  );
}

export function createAppRouter() {
  const routerOptions = {
    future: {
      v7_relativeSplatPath: true,
      v7_startTransition: true,
    },
  } as Parameters<typeof createHashRouter>[1];

  return createHashRouter(
    [
      {
        path: '/',
        element: <AdminLayout />,
        children: [
          { index: true, element: <HomeRedirect /> },
          {
            path: 'dashboard',
            element: (
              <ProtectedModuleRoute moduleKey="dashboard">
                <DashboardPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'users',
            element: (
              <ProtectedModuleRoute moduleKey="users">
                <UsersPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'operators',
            element: (
              <ProtectedModuleRoute moduleKey="operators">
                <OperatorsPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'orders',
            element: (
              <ProtectedModuleRoute moduleKey="orders">
                <OrdersPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'wallet',
            element: (
              <ProtectedModuleRoute moduleKey="wallet">
                <WalletPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'catalog',
            element: (
              <ProtectedModuleRoute moduleKey="catalog">
                <CatalogPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'audits',
            element: (
              <ProtectedModuleRoute moduleKey="audits">
                <AuditsPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'refunds',
            element: (
              <ProtectedModuleRoute moduleKey="refunds">
                <RefundsPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'infringement',
            element: (
              <ProtectedModuleRoute moduleKey="infringement">
                <InfringementPage />
              </ProtectedModuleRoute>
            ),
          },
          {
            path: 'governance',
            element: (
              <ProtectedModuleRoute moduleKey="governance">
                <GovernancePage />
              </ProtectedModuleRoute>
            ),
          },
          { path: '*', element: <HomeRedirect /> },
        ],
      },
    ],
    routerOptions,
  );
}
