import { useState } from 'react';

import { MetricCard } from '../../components/display/MetricCard';
import { StatusBadge } from '../../components/display/StatusBadge';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { PageHeader } from '../../components/layout/PageHeader';
import { SimpleTrendPanel } from '../../components/charts/SimpleTrendPanel';
import { buildDashboardMetrics, buildOperationalHighlights, buildTopModelItems } from './dashboard.types';
import { useAdminDashboard } from './dashboard.query';

const windowOptions = [7, 30, 90] as const;

export function DashboardPage() {
  const [windowDays, setWindowDays] = useState<(typeof windowOptions)[number]>(30);
  const dashboardQuery = useAdminDashboard(windowDays);
  const metrics = buildDashboardMetrics(dashboardQuery.data);
  const topModels = buildTopModelItems(dashboardQuery.data);
  const highlights = buildOperationalHighlights(dashboardQuery.data);

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="平台总览"
        title="平台仪表盘"
        description="围绕用户、财务、模型与治理建立统一运营视图，帮助管理员快速判断平台状态、资金变化与待处理风险。"
        meta={
          <>
            <StatusBadge tone="success">后台统计已连接</StatusBadge>
            <StatusBadge tone="info">{`时间窗 ${windowDays} 天`}</StatusBadge>
          </>
        }
        actions={
          <form className="filter-bar" id="dashboardFilterForm">
            <span className="filter-bar__label">观察窗口</span>
            <div className="segmented-control" id="dashboardFilterSinceDays" role="group" aria-label="观察窗口">
              {windowOptions.map(option => (
                <button
                  className={['segmented-control__button', option === windowDays ? 'is-active' : ''].filter(Boolean).join(' ')}
                  key={option}
                  onClick={() => setWindowDays(option)}
                  type="button"
                >
                  {`最近 ${option} 天`}
                </button>
              ))}
            </div>
          </form>
        }
      />

      {dashboardQuery.isError ? (
        <InlineStatus tone="warning">
          {dashboardQuery.error instanceof Error ? dashboardQuery.error.message : '仪表盘数据加载失败。'}
        </InlineStatus>
      ) : (
        <InlineStatus tone={dashboardQuery.isFetching ? 'info' : 'success'}>
          {dashboardQuery.isFetching ? '正在同步后台仪表盘数据…' : '仪表盘数据已同步，可结合订单、钱包和治理模块继续下钻排查。'}
        </InlineStatus>
      )}

      <div className="metric-grid">
        {metrics.map(metric => (
          <MetricCard key={metric.label} caption={metric.caption} label={metric.label} tone={metric.tone} trend={metric.trend} value={metric.value} />
        ))}
      </div>

      <div className="panel-grid panel-grid--balanced">
        <SimpleTrendPanel title="模型消耗排行" description="展示当前窗口内收费最高的官方模型，帮助运营快速对照价格与路由配置。" items={topModels} />
        <SimpleTrendPanel title="运营提醒" description="把资金、模型和治理三个方向的高优先级信号收束到一个视图。" items={highlights} />
      </div>
    </section>
  );
}
