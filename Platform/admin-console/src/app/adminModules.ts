import type { StatusTone } from '../components/display/StatusBadge';
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

export type PlaceholderPageConfig = {
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

export const placeholderPages: Record<
  Exclude<AdminModuleKey, 'dashboard' | 'users' | 'orders' | 'wallet'>,
  PlaceholderPageConfig
> = {
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
