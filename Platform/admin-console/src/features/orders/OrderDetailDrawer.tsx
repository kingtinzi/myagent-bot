import { DetailDrawer } from '../../components/data/DetailDrawer';
import type { RechargeOrder } from '../../services/contracts';
import { ReconcileActionBar } from './ReconcileActionBar';

type OrderDetailDrawerProps = {
  onClose: () => void;
  onReconcile: (order: RechargeOrder) => Promise<void>;
  open: boolean;
  order: RechargeOrder | null;
};

export function OrderDetailDrawer({ onClose, onReconcile, open, order }: OrderDetailDrawerProps) {
  return (
    <DetailDrawer
      actions={order ? <ReconcileActionBar onReconcile={() => onReconcile(order)} /> : undefined}
      description={order ? `${order.provider || '未指定渠道'} · ${order.status}` : '订单详情'}
      onClose={onClose}
      open={open}
      title={order?.id || '订单详情'}
    >
      {order ? (
        <dl className="detail-meta-list">
          <div>
            <dt>用户 ID</dt>
            <dd>{order.user_id}</dd>
          </div>
          <div>
            <dt>金额</dt>
            <dd>{order.amount_fen.toLocaleString('zh-CN')} 分</dd>
          </div>
          <div>
            <dt>状态</dt>
            <dd>{order.status}</dd>
          </div>
          <div>
            <dt>外部单号</dt>
            <dd>{order.external_id || '—'}</dd>
          </div>
        </dl>
      ) : null}
    </DetailDrawer>
  );
}
