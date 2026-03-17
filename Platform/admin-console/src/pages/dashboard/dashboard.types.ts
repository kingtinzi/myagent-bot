import type { AdminDashboard } from '../../services/contracts';
import type { StatusTone } from '../../services/contracts';

export type DashboardMetric = {
  label: string;
  value: string;
  caption: string;
  trend: string;
  tone: Exclude<StatusTone, 'neutral'>;
};

export type TrendPanelItem = {
  title: string;
  description: string;
  badge: string;
  tone: Exclude<StatusTone, 'neutral'>;
};

const currencyFormatter = new Intl.NumberFormat('zh-CN', {
  style: 'currency',
  currency: 'CNY',
  minimumFractionDigits: 2,
});

const integerFormatter = new Intl.NumberFormat('zh-CN');

function formatCurrencyFen(amountFen: number) {
  return currencyFormatter.format(amountFen / 100);
}

function formatInteger(value: number) {
  return integerFormatter.format(value);
}

export function buildDashboardMetrics(dashboard?: AdminDashboard): DashboardMetric[] {
  if (!dashboard) {
    return [
      { label: '活跃用户', value: '—', caption: '正在同步后台统计', trend: '加载中', tone: 'info' },
      { label: '已支付订单', value: '—', caption: '正在同步后台统计', trend: '加载中', tone: 'info' },
      { label: '钱包余额', value: '—', caption: '正在同步后台统计', trend: '加载中', tone: 'info' },
      { label: '待处理风险', value: '—', caption: '正在同步后台统计', trend: '加载中', tone: 'info' },
    ];
  }

  return [
    {
      label: '活跃用户',
      value: formatInteger(dashboard.totals.users),
      caption: `最近 ${dashboard.recent.window_days ?? 7} 天新增 ${formatInteger(dashboard.recent.new_users_7d)} 位用户`,
      trend: '用户池',
      tone: 'success',
    },
    {
      label: '已支付订单',
      value: formatInteger(dashboard.totals.paid_orders),
      caption: `窗口内充值 ${formatCurrencyFen(dashboard.recent.recharge_fen_7d)}`,
      trend: '资金流',
      tone: 'info',
    },
    {
      label: '钱包余额',
      value: formatCurrencyFen(dashboard.totals.wallet_balance_fen),
      caption: `窗口内消耗 ${formatCurrencyFen(dashboard.recent.consumption_fen_7d)}`,
      trend: '资产池',
      tone: 'warning',
    },
    {
      label: '待处理风险',
      value: formatInteger(dashboard.totals.refund_pending + dashboard.totals.infringement_pending),
      caption: `退款 ${formatInteger(dashboard.totals.refund_pending)} / 侵权 ${formatInteger(dashboard.totals.infringement_pending)}`,
      trend: '需处理',
      tone: 'danger',
    },
  ];
}

export function buildTopModelItems(dashboard?: AdminDashboard): TrendPanelItem[] {
  if (!dashboard || dashboard.top_models.length === 0) {
    return [
      {
        title: '暂无模型数据',
        description: '当前时间窗内暂无官方模型消耗记录，产生调用后会在这里展示排行与 token 用量。',
        badge: '等待数据',
        tone: 'info',
      },
    ];
  }

  return dashboard.top_models.slice(0, 3).map(item => ({
    title: item.model_id,
    description: `调用 ${formatInteger(item.usage_count)} 次 · 收费 ${formatCurrencyFen(item.charged_fen)} · Prompt ${formatInteger(item.prompt_tokens)} / Completion ${formatInteger(item.completion_tokens)}`,
    badge: formatCurrencyFen(item.charged_fen),
    tone: 'info',
  }));
}

export function buildOperationalHighlights(dashboard?: AdminDashboard): TrendPanelItem[] {
  if (!dashboard) {
    return [
      {
        title: '正在同步运营提醒',
        description: '系统正在汇总充值、模型和治理指标，完成后会在这里显示最新提醒。',
        badge: '同步中',
        tone: 'info',
      },
    ];
  }

  return [
    {
      title: '支付链路',
      description: `最近窗口充值 ${formatCurrencyFen(dashboard.recent.recharge_fen_7d)}，需要结合订单页继续排查异常支付。`,
      badge: dashboard.recent.recharge_fen_7d > 0 ? '正常' : '无新增',
      tone: dashboard.recent.recharge_fen_7d > 0 ? 'success' : 'warning',
    },
    {
      title: '模型用量',
      description: `最近窗口消耗 ${formatCurrencyFen(dashboard.recent.consumption_fen_7d)}，可继续下钻到目录与价格规则。`,
      badge: dashboard.top_models.length > 0 ? `${dashboard.top_models.length} 个模型` : '0 个模型',
      tone: dashboard.top_models.length > 0 ? 'info' : 'warning',
    },
    {
      title: '治理风险',
      description: `当前共有 ${formatInteger(dashboard.totals.refund_pending + dashboard.totals.infringement_pending)} 条待处理风险事项。`,
      badge: dashboard.totals.refund_pending + dashboard.totals.infringement_pending > 0 ? '待处理' : '清空',
      tone: dashboard.totals.refund_pending + dashboard.totals.infringement_pending > 0 ? 'danger' : 'success',
    },
  ];
}
