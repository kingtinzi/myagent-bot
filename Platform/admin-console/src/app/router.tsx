import type { ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Navigate, Outlet, createBrowserRouter } from 'react-router-dom';

import { EmptyState } from '../components/display/EmptyState';
import { StatusBadge, type StatusTone } from '../components/display/StatusBadge';
import { InlineStatus } from '../components/feedback/InlineStatus';
import { AdminShell, type AdminShellOperator, type AdminShellToast } from '../components/layout/AdminShell';
import type { AdminNavigationGroup } from '../components/layout/AdminSidebar';
import { PageHeader } from '../components/layout/PageHeader';
import { DashboardPage } from '../pages/dashboard/DashboardPage';
import { OrdersPage } from '../pages/orders/OrdersPage';
import { UsersPage } from '../pages/users/UsersPage';
import { WalletPage } from '../pages/wallet/WalletPage';

type PlaceholderPageConfig = {
  eyebrow: string;
  title: string;
  description: string;
  spotlightLabel: string;
  spotlightTone: StatusTone;
  checklistTitle: string;
  checklist: string[];
  summaryTitle: string;
  summary: string;
  actionLabel: string;
};

const navigationGroups: AdminNavigationGroup[] = [
  {
    id: 'overview',
    label: '总览',
    items: [
      { id: 'dashboard', to: '/dashboard', label: '仪表盘', description: '关键指标、活跃度与风险总览' },
      { id: 'users', to: '/users', label: '用户', description: '账号、钱包与协议签署记录' },
      { id: 'operators', to: '/operators', label: '管理员', description: '角色、权限与后台值班配置' },
    ],
  },
  {
    id: 'finance',
    label: '财务与目录',
    items: [
      { id: 'orders', to: '/orders', label: '订单', description: '充值订单、支付链路与对账' },
      { id: 'wallet', to: '/wallet', label: '钱包', description: '手动充值、调账与账本流水' },
      { id: 'catalog', to: '/catalog', label: '目录', description: '模型、路由、价格与协议版本', badge: '核心', badgeTone: 'info' },
    ],
  },
  {
    id: 'governance',
    label: '控制与审核',
    items: [
      { id: 'audits', to: '/audits', label: '审计', description: '敏感操作轨迹与导出审计' },
      { id: 'refunds', to: '/refunds', label: '退款', description: '退款审核、驳回与结算' },
      { id: 'infringement', to: '/infringement', label: '侵权', description: '案件处理、证据与结论' },
      { id: 'governance', to: '/governance', label: '治理', description: '公告、风控规则与保留策略', badge: '策略', badgeTone: 'warning' },
    ],
  },
];

const operatorProfile: AdminShellOperator = {
  displayName: '俊夫',
  meta: '超级管理员 · 平台治理 / 财务 / 内容审核',
  roleLabel: '超级管理员',
  roleTone: 'success',
};

const shellToast: AdminShellToast = {
  tone: 'info',
  title: '后台重构进行中',
  message: '新版管理后台壳层已建立，后续会分批切换用户、钱包、模型与治理模块。',
};

const placeholderPages: Record<string, PlaceholderPageConfig> = {
  users: {
    eyebrow: '账号中心',
    title: '用户与账号',
    description: '统一管理用户身份、注册协议签署、余额概览与高级行为轨迹。',
    spotlightLabel: '优先迁移',
    spotlightTone: 'success',
    checklistTitle: '当前页会重点重建',
    checklist: ['筛选条 + 用户表格 + 详情抽屉', '优先显示用户名 / 用户编号 / 邮箱', '在同一视图串联协议、钱包、订单和用量'],
    summaryTitle: '迁移策略',
    summary: '保留现有服务端权限模型与详情接口，前端升级为多面板协同视图，避免继续在单文件里堆砌状态。',
    actionLabel: '查看用户域迁移清单',
  },
  operators: {
    eyebrow: '权限治理',
    title: '管理员与权限',
    description: '面向运营与超级管理员的权限分配、角色梳理与高风险操作确认体验。',
    spotlightLabel: '高风险',
    spotlightTone: 'warning',
    checklistTitle: '本页设计目标',
    checklist: ['角色差异一屏可读', '危险动作统一二次确认', '权限开关、说明与审计痕迹同源展示'],
    summaryTitle: '迁移策略',
    summary: '先复用现有管理员 session 与 capability 返回值，再重构表单、角色说明和危险操作交互，确保前后端权限边界不漂移。',
    actionLabel: '预览角色矩阵',
  },
  orders: {
    eyebrow: '交易运营',
    title: '充值订单',
    description: '覆盖创建、支付、超时、回调与对账状态，帮助运营快速定位异常订单。',
    spotlightLabel: '支付链路',
    spotlightTone: 'info',
    checklistTitle: '订单页重构重点',
    checklist: ['统一状态筛选、时间窗与用户维度过滤', '异常订单单独标识并支持快速追溯', '为人工入账 / 回调异常预留处理入口'],
    summaryTitle: '迁移策略',
    summary: '该页会遵循“筛选条 + 表格 + 详情抽屉”的新信息架构，优先解决旧版订单状态冗余与操作上下文割裂的问题。',
    actionLabel: '查看订单迁移方案',
  },
  wallet: {
    eyebrow: '财务中台',
    title: '钱包与手动充值',
    description: '用于管理员手动充值、调账、查看余额变化与消费流水，强调确认链路与审计可追溯。',
    spotlightLabel: '人工充值',
    spotlightTone: 'success',
    checklistTitle: '钱包页核心能力',
    checklist: ['手动充值与调账分区展示', '上下文用户切换后自动联动表单与流水', '所有财务动作保留强确认与反馈状态'],
    summaryTitle: '迁移策略',
    summary: '保留现有手动充值与调账接口，把表单、账户上下文横幅和流水历史拆成更清晰的任务流，降低误操作概率。',
    actionLabel: '查看充值工作流',
  },
  catalog: {
    eyebrow: '模型目录',
    title: '官方模型与协议目录',
    description: '集中管理官方模型路由、协议类型、价格规则、协议版本与充值前知情材料。',
    spotlightLabel: '核心配置',
    spotlightTone: 'info',
    checklistTitle: '目录页结构',
    checklist: ['模型目录、上游路由、价格规则拆分为多个编辑面板', '支持“协议类型 / 调用方式”显式配置', '协议发布与价格变更需要风险提示'],
    summaryTitle: '迁移策略',
    summary: '旧版高级编辑器会逐步替换为结构化表单 + 安全 JSON 预览，保留 revision 并发保护，避免配置覆盖。',
    actionLabel: '预览模型配置结构',
  },
  audits: {
    eyebrow: '安全审计',
    title: '审计日志',
    description: '提供敏感操作检索、风险级别筛选、时间范围过滤与 CSV 导出能力。',
    spotlightLabel: '审计必达',
    spotlightTone: 'danger',
    checklistTitle: '日志页重点',
    checklist: ['时间窗、风险级别、目标对象联合筛选', '审计导出明确权限与导出提示', '详情视图保留操作前后摘要与操作者信息'],
    summaryTitle: '迁移策略',
    summary: '新 UI 会强化日志过滤和导出反馈，同时保持服务端鉴权、CSV 导出接口和风险等级模型不变。',
    actionLabel: '查看审计筛选器',
  },
  refunds: {
    eyebrow: '退款审核',
    title: '退款中心',
    description: '聚合退款申请、外部退款状态、审核备注与结算动作，强化审批节奏感。',
    spotlightLabel: '需审批',
    spotlightTone: 'warning',
    checklistTitle: '退款页目标',
    checklist: ['列表右侧保留审核详情与操作区', '审批前提供完整风险提示与恢复建议', '外部状态、单号与内部账本联动展示'],
    summaryTitle: '迁移策略',
    summary: '将旧版退款审批的分散字段整合为抽屉式工作台，降低运营在高风险动作中的认知负担。',
    actionLabel: '预览退款工作台',
  },
  infringement: {
    eyebrow: '风控案件',
    title: '侵权与投诉处理',
    description: '处理侵权举报、证据链接、状态演进与结论备注，确保留痕清晰、链接安全。',
    spotlightLabel: '风控案件',
    spotlightTone: 'danger',
    checklistTitle: '侵权页重点',
    checklist: ['案件列表、证据区域与处理结论分栏展示', '不安全外链默认拦截并给出提示', '保留完整审计痕迹与时间线'],
    summaryTitle: '迁移策略',
    summary: '新版将突出案件上下文和处理流，优先强化证据阅读与状态切换时的风险反馈，减少误判。',
    actionLabel: '查看案件视图草案',
  },
  governance: {
    eyebrow: '平台治理',
    title: '公告、协议与保留策略',
    description: '管理公告发布、协议版本、知情文案、风控规则与数据保留策略。',
    spotlightLabel: '策略控制',
    spotlightTone: 'warning',
    checklistTitle: '治理页核心能力',
    checklist: ['协议、公告、保留策略按对象分区', '变更时显示对线上用户的直接影响', '危险操作与发布行为保留审计确认'],
    summaryTitle: '迁移策略',
    summary: '这一页将成为“面向线上行为的控制台”，强调发布时间、影响范围和回滚提示，而不再只是原始 JSON 编辑器。',
    actionLabel: '查看治理对象分区',
  },
};

function QueryProviderProbe() {
  const queryClient = useQueryClient();

  return (
    <output hidden data-testid="query-provider-ready">
      {queryClient ? 'ready' : 'missing'}
    </output>
  );
}

function AdminLayout() {
  return (
    <AdminShell
      navigationGroups={navigationGroups}
      operator={operatorProfile}
      toast={shellToast}
      onRefresh={() => undefined}
      onSignOut={() => undefined}
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
              <p>保持极简企业 SaaS 风格，但让高频工作流更快、更稳、更容易审查。</p>
            </div>
            <StatusBadge tone={config.spotlightTone}>正在设计</StatusBadge>
          </div>
          <div className="list-grid">
            {config.checklist.map(item => (
              <InfoRow key={item} title={item} description="基于现有后端接口与权限模型逐项收口，不做临时补丁式拼接。" />
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
  return <ModulePlaceholderPage config={placeholderPages[moduleID]} />;
}

export function createAppRouter() {
  const routerOptions = {
    future: {
      v7_relativeSplatPath: true,
      v7_startTransition: true,
    },
  } as Parameters<typeof createBrowserRouter>[1];

  return createBrowserRouter([
    {
      path: '/',
      element: <AdminLayout />,
      children: [
        { index: true, element: <Navigate to="/dashboard" replace /> },
        { path: 'dashboard', element: <DashboardPage /> },
        { path: 'users', element: <UsersPage /> },
        { path: 'operators', element: placeholderRoute('operators') },
        { path: 'orders', element: <OrdersPage /> },
        { path: 'wallet', element: <WalletPage /> },
        { path: 'catalog', element: placeholderRoute('catalog') },
        { path: 'audits', element: placeholderRoute('audits') },
        { path: 'refunds', element: placeholderRoute('refunds') },
        { path: 'infringement', element: placeholderRoute('infringement') },
        { path: 'governance', element: placeholderRoute('governance') },
      ],
    },
  ], routerOptions);
}
