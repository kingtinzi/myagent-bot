import { StatusBadge } from '../../components/display/StatusBadge';
import type { AdminUserOverview } from '../../services/contracts';

type UserOverviewTabProps = {
  overview: AdminUserOverview;
};

export function UserOverviewTab({ overview }: UserOverviewTabProps) {
  return (
    <div className="detail-tab-stack">
      <div className="metric-grid metric-grid--compact">
        <article className="metric-card is-info">
          <div className="metric-card__header">
            <span>余额</span>
            <StatusBadge tone="info">{overview.wallet.currency}</StatusBadge>
          </div>
          <strong>{(overview.wallet.balance_fen / 100).toFixed(2)}</strong>
          <p>当前钱包余额（元）</p>
        </article>
        <article className="metric-card is-success">
          <div className="metric-card__header">
            <span>订单</span>
            <StatusBadge tone="success">{overview.recent_orders.length} 条</StatusBadge>
          </div>
          <strong>{overview.user.order_count ?? overview.recent_orders.length}</strong>
          <p>累计订单数量</p>
        </article>
        <article className="metric-card is-warning">
          <div className="metric-card__header">
            <span>待处理退款</span>
            <StatusBadge tone="warning">风险</StatusBadge>
          </div>
          <strong>{overview.pending_refund_count}</strong>
          <p>需要跟进的退款申请</p>
        </article>
        <article className="metric-card is-danger">
          <div className="metric-card__header">
            <span>侵权案件</span>
            <StatusBadge tone="danger">治理</StatusBadge>
          </div>
          <strong>{overview.pending_infringement_count}</strong>
          <p>待审核的侵权举报</p>
        </article>
      </div>
    </div>
  );
}
