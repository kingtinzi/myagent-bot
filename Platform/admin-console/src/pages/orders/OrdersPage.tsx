import { useMemo, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { DataTable } from '../../components/data/DataTable';
import { FilterBar } from '../../components/data/FilterBar';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { PageHeader } from '../../components/layout/PageHeader';
import { useCapabilities } from '../../hooks/useCapabilities';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { RechargeOrder } from '../../services/contracts';
import { OrderDetailDrawer } from '../../features/orders/OrderDetailDrawer';

type OrdersLocationState = {
  userId?: string;
};

export function OrdersPage() {
  const location = useLocation();
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const capabilities = useCapabilities();
  const locationState = (location.state ?? {}) as OrdersLocationState;
  const [draftKeyword, setDraftKeyword] = useState(locationState.userId ?? '');
  const [keyword, setKeyword] = useState(locationState.userId ?? '');
  const [status, setStatus] = useState('');
  const [selectedOrder, setSelectedOrder] = useState<RechargeOrder | null>(null);

  const ordersQuery = useQuery({
    queryKey: ['admin', 'orders', keyword, status],
    queryFn: () =>
      adminApi.listOrders({
        userId: keyword.trim() || undefined,
        keyword: keyword.trim() || undefined,
        status: status || undefined,
      }),
    retry: false,
  });

  const reconcileMutation = useMutation({
    mutationFn: async (order: RechargeOrder) => {
      const confirmed = await confirmAction({
        title: '订单对账',
        message: `即将对账订单 ${order.id}。`,
        hint: `当前状态 ${order.status}，支付渠道 ${order.provider || '未指定'}`,
        confirmLabel: '确认对账',
        tone: 'warning',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.reconcileOrder(order.id);
    },
    onSuccess: result => {
      if (!result) {
        return;
      }
      setSelectedOrder(result.order);
      void queryClient.invalidateQueries({ queryKey: ['admin', 'orders'] });
    },
  });

  const columns = useMemo(
    () => [
      { key: 'id', header: '订单号', cell: (row: RechargeOrder) => row.id },
      { key: 'user', header: '用户 ID', cell: (row: RechargeOrder) => row.user_id },
      { key: 'status', header: '状态', cell: (row: RechargeOrder) => row.status },
      { key: 'amount', header: '金额（分）', cell: (row: RechargeOrder) => row.amount_fen.toLocaleString('zh-CN') },
      { key: 'provider', header: '支付渠道', cell: (row: RechargeOrder) => row.provider || '—' },
    ],
    [],
  );

  if (!capabilities.canAccessModule('orders')) {
    return <InlineStatus tone="warning">当前管理员没有查看订单模块的权限。</InlineStatus>;
  }

  return (
    <section className="page-stack">
      <PageHeader eyebrow="交易运营" title="充值订单" description="按用户、状态查看订单，并对异常订单执行手动对账。" />

      <FilterBar
        onSubmit={event => {
          event.preventDefault();
          setKeyword(draftKeyword.trim());
        }}
        actions={
          <button className="button button--primary" type="submit">
            查询订单
          </button>
        }
      >
        <label className="filter-field">
          <span>用户关键字</span>
          <input onChange={event => setDraftKeyword(event.target.value)} placeholder="用户 ID / 用户名 / 编号" value={draftKeyword} />
        </label>
        <label className="filter-field">
          <span>状态</span>
          <select onChange={event => setStatus(event.target.value)} value={status}>
            <option value="">全部状态</option>
            <option value="pending">pending</option>
            <option value="paid">paid</option>
            <option value="refunded">refunded</option>
          </select>
        </label>
      </FilterBar>

      <InlineStatus tone={ordersQuery.isFetching ? 'info' : 'success'}>
        {ordersQuery.isFetching ? '正在加载订单数据…' : '点击订单行可打开详情并执行手动对账。'}
      </InlineStatus>

      <DataTable
        caption="订单列表"
        columns={columns}
        emptyMessage="当前没有符合条件的订单。"
        getRowKey={row => row.id}
        onRowClick={row => setSelectedOrder(row)}
        rows={ordersQuery.data ?? []}
      />

      <OrderDetailDrawer
        onClose={() => setSelectedOrder(null)}
        onReconcile={order => reconcileMutation.mutateAsync(order).then(() => undefined)}
        open={Boolean(selectedOrder)}
        order={selectedOrder}
      />
    </section>
  );
}
