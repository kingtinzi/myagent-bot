import { DataTable } from '../../components/data/DataTable';
import type { RechargeOrder } from '../../services/contracts';

type UserOrdersTabProps = {
  orders: RechargeOrder[];
};

export function UserOrdersTab({ orders }: UserOrdersTabProps) {
  return (
    <DataTable
      caption="用户订单记录"
      columns={[
        { key: 'id', header: '订单号', cell: row => row.id },
        { key: 'status', header: '状态', cell: row => row.status },
        { key: 'amount', header: '金额（分）', cell: row => row.amount_fen.toLocaleString('zh-CN') },
        { key: 'provider', header: '支付渠道', cell: row => row.provider || '—' },
      ]}
      getRowKey={row => row.id}
      rows={orders}
    />
  );
}
