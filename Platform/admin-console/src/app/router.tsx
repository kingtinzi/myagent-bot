import type { ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Navigate, Outlet, createHashRouter, useLocation } from 'react-router-dom';

import { buildNavigationGroups, firstAccessiblePath, placeholderPages, type PlaceholderPageConfig } from './adminModules';
import { EmptyState } from '../components/display/EmptyState';
import { StatusBadge, type StatusTone } from '../components/display/StatusBadge';
import { InlineStatus } from '../components/feedback/InlineStatus';
import { AdminShell, type AdminShellOperator } from '../components/layout/AdminShell';
import { PageHeader } from '../components/layout/PageHeader';
import { useAdminSession } from '../hooks/useAdminSession';
import { useCapabilities, type AdminModuleKey } from '../hooks/useCapabilities';
import type { AdminSession } from '../services/contracts';
import { DashboardPage } from '../pages/dashboard/DashboardPage';
import { CatalogPage } from '../pages/catalog/CatalogPage';
import { GovernancePage } from '../pages/governance/GovernancePage';
import { OrdersPage } from '../pages/orders/OrdersPage';
import { OperatorsPage } from '../pages/operators/OperatorsPage';
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

function ModulePlaceholderPage({ config }: { config: PlaceholderPageConfig }) {
  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={config.eyebrow}
        title={config.title}
        description={config.description}
        meta={<StatusBadge tone={config.spotlightTone}>{config.spotlightLabel}</StatusBadge>}
        actions={
          <>
            <button className="button button--ghost" type="button">
              查看旧版流程
            </button>
            <button className="button button--primary" type="button">
              {config.actionLabel}
            </button>
          </>
        }
      />

      <div className="panel-grid panel-grid--balanced">
        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>{config.checklistTitle}</h2>
              <p>保持极简企业 SaaS 风格，同时让高频工作流更快、更稳、更容易审查。</p>
            </div>
            <StatusBadge tone={config.spotlightTone}>正在设计</StatusBadge>
          </div>
          <div className="list-grid">
            {config.checklist.map(item => (
              <InfoRow key={item} description="基于现有后端接口与权限模型逐项收口，不做临时补丁式拼接。" title={item} />
            ))}
          </div>
        </section>

        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>{config.summaryTitle}</h2>
              <p>确保前端重构不影响现有管理员工作流，同时为后续模块拆分留出空间。</p>
            </div>
          </div>
          <InlineStatus tone={config.spotlightTone}>{config.summary}</InlineStatus>
          <EmptyState
            eyebrow="模块迁移中"
            title={`${config.title}将在下一阶段接入真实数据`}
            description="当前页面主要用于确认信息架构、交互层级与危险操作反馈方式，后续会直接绑定后台 API。"
          />
        </section>
      </div>
    </section>
  );
}

function InfoRow({
  title,
  description,
  badge,
}: {
  title: string;
  description: string;
  badge?: ReactNode;
}) {
  return (
    <article className="info-row">
      <div className="info-row__copy">
        <strong>{title}</strong>
        <p>{description}</p>
      </div>
      {badge ? <div className="info-row__badge">{badge}</div> : null}
    </article>
  );
}

function placeholderRoute(moduleID: keyof typeof placeholderPages) {
  return (
    <ProtectedModuleRoute moduleKey={moduleID}>
      <ModulePlaceholderPage config={placeholderPages[moduleID]} />
    </ProtectedModuleRoute>
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
          { path: 'audits', element: placeholderRoute('audits') },
          { path: 'refunds', element: placeholderRoute('refunds') },
          { path: 'infringement', element: placeholderRoute('infringement') },
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
