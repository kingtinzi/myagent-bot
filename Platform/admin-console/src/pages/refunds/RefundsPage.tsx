import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { DataTable } from '../../components/data/DataTable';
import { StatusBadge } from '../../components/display/StatusBadge';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { PageHeader } from '../../components/layout/PageHeader';
import { PermissionGate } from '../../components/navigation/PermissionGate';
import { useCapabilities } from '../../hooks/useCapabilities';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { AdminRefundDecisionInput, RefundRequest } from '../../services/contracts';

type RefundFilterDraft = {
  userId: string;
  keyword: string;
  orderId: string;
  status: string;
};

type RefundAction = 'approve' | 'reject' | 'settle';

function createFilterDraft(): RefundFilterDraft {
  return {
    userId: '',
    keyword: '',
    orderId: '',
    status: '',
  };
}

function formatTimestamp(value?: number) {
  if (!value) {
    return '—';
  }

  return new Date(value * 1000).toLocaleString('zh-CN', {
    hour12: false,
  });
}

function formatMoney(value: number) {
  return `¥${(value / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function statusTone(status: string | undefined): 'neutral' | 'info' | 'success' | 'warning' | 'danger' {
  switch ((status ?? '').trim()) {
    case 'rejected':
    case 'refund_failed':
      return 'danger';
    case 'approved_pending_payout':
      return 'warning';
    case 'refunded':
      return 'success';
    case 'pending':
      return 'info';
    default:
      return 'neutral';
  }
}

function statusLabel(status: string | undefined) {
  switch ((status ?? '').trim()) {
    case 'pending':
      return '待审核';
    case 'rejected':
      return '已驳回';
    case 'approved_pending_payout':
      return '待打款';
    case 'refunded':
      return '已退款';
    case 'refund_failed':
      return '退款失败';
    default:
      return status?.trim() || '未知状态';
  }
}

function toQuery(draft: RefundFilterDraft) {
  return {
    userId: draft.userId.trim() || undefined,
    keyword: draft.keyword.trim() || undefined,
    orderId: draft.orderId.trim() || undefined,
    status: draft.status.trim() || undefined,
    limit: 50,
  };
}

export function RefundsPage() {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const capabilities = useCapabilities();
  const [filterDraft, setFilterDraft] = useState<RefundFilterDraft>(() => createFilterDraft());
  const [appliedFilters, setAppliedFilters] = useState(() => toQuery(createFilterDraft()));
  const [selectedRefundID, setSelectedRefundID] = useState('');
  const [reviewNote, setReviewNote] = useState('');
  const [refundProvider, setRefundProvider] = useState('');
  const [externalRefundId, setExternalRefundId] = useState('');
  const [externalStatus, setExternalStatus] = useState('');
  const [statusMessage, setStatusMessage] = useState('退款中心统一处理待审核、待打款和失败退款的闭环。');

  const refundsQuery = useQuery({
    queryKey: ['admin', 'refunds', appliedFilters],
    queryFn: () => adminApi.listRefundRequests(appliedFilters),
    retry: false,
  });

  const selectedRefund = useMemo(
    () => refundsQuery.data?.find(item => item.id === selectedRefundID) ?? null,
    [refundsQuery.data, selectedRefundID],
  );

  useEffect(() => {
    const items = refundsQuery.data ?? [];

    if (items.length === 0) {
      setSelectedRefundID('');
      return;
    }

    const next = items.find(item => item.id === selectedRefundID) ?? items[0];
    setSelectedRefundID(next.id);
  }, [refundsQuery.data, selectedRefundID]);

  useEffect(() => {
    if (!selectedRefund) {
      setReviewNote('');
      setRefundProvider('');
      setExternalRefundId('');
      setExternalStatus('');
      return;
    }

    setReviewNote(selectedRefund.review_note ?? '');
    setRefundProvider(selectedRefund.refund_provider ?? '');
    setExternalRefundId(selectedRefund.external_refund_id ?? '');
    setExternalStatus(selectedRefund.external_status ?? '');
  }, [selectedRefund?.id]);

  const actionMutation = useMutation({
    mutationFn: async ({ action, refund }: { action: RefundAction; refund: RefundRequest }) => {
      const actionConfig = {
        approve: { title: '通过退款', hint: '系统会尝试调用退款能力；若通道不支持，会自动进入待打款状态。' },
        reject: { title: '驳回退款', hint: '驳回后需要在备注中保留明确原因，方便用户申诉与内部审计。' },
        settle: { title: '结算退款', hint: '仅在已完成线下/外部打款后执行结算，请确认外部退款单号和状态。' },
      }[action];

      const confirmed = await confirmAction({
        title: `${actionConfig.title}：${refund.id}`,
        message: `即将处理退款请求 ${refund.id}，金额 ${formatMoney(refund.amount_fen)}。`,
        hint: `${actionConfig.hint} 请输入 CONFIRM 继续。`,
        confirmLabel: actionConfig.title,
        tone: 'danger',
        requireText: 'CONFIRM',
      });

      if (!confirmed) {
        return null;
      }

      const payload: AdminRefundDecisionInput = {
        review_note: reviewNote.trim() || undefined,
        refund_provider: refundProvider.trim() || undefined,
        external_refund_id: externalRefundId.trim() || undefined,
        external_status: externalStatus.trim() || undefined,
      };

      switch (action) {
        case 'approve':
          return adminApi.approveRefundRequest(refund.id, payload);
        case 'reject':
          return adminApi.rejectRefundRequest(refund.id, payload);
        case 'settle':
          return adminApi.settleRefundRequest(refund.id, payload);
      }
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('退款操作已取消。');
        return;
      }
      setSelectedRefundID(result.id);
      setStatusMessage(`退款请求 ${result.id} 已更新为「${statusLabel(result.status)}」。`);
      void queryClient.invalidateQueries({ queryKey: ['admin', 'refunds'] });
    },
  });

  const columns = useMemo(
    () => [
      {
        key: 'id',
        header: '退款单',
        cell: (row: RefundRequest) => (
          <div className="user-identity-cell">
            <strong>{row.id}</strong>
            <small>{row.order_id}</small>
          </div>
        ),
      },
      {
        key: 'user',
        header: '用户',
        cell: (row: RefundRequest) => (
          <div className="user-identity-cell">
            <strong>{row.username || row.user_no || row.user_id}</strong>
            <small>{row.user_id}</small>
          </div>
        ),
      },
      {
        key: 'amount',
        header: '金额',
        cell: (row: RefundRequest) => formatMoney(row.amount_fen),
      },
      {
        key: 'status',
        header: '状态',
        cell: (row: RefundRequest) => <StatusBadge tone={statusTone(row.status)}>{statusLabel(row.status)}</StatusBadge>,
      },
      {
        key: 'updated',
        header: '最近更新',
        cell: (row: RefundRequest) => formatTimestamp(row.updated_unix),
      },
    ],
    [],
  );

  const statusToneValue = refundsQuery.isError || actionMutation.isError ? 'danger' : refundsQuery.isFetching ? 'info' : 'success';
  const statusText = refundsQuery.isError
    ? refundsQuery.error instanceof Error
      ? refundsQuery.error.message
      : '加载退款请求失败，请稍后重试。'
    : actionMutation.isError
      ? actionMutation.error instanceof Error
        ? actionMutation.error.message
        : '执行退款操作失败，请稍后重试。'
      : refundsQuery.isFetching
        ? '正在加载退款请求…'
        : statusMessage;

  function applyFilters() {
    setAppliedFilters(toQuery(filterDraft));
    setStatusMessage('已应用退款筛选条件。');
  }

  function resetFilters() {
    const next = createFilterDraft();
    setFilterDraft(next);
    setAppliedFilters(toQuery(next));
    setStatusMessage('已重置退款筛选条件。');
  }

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="退款中心"
        title="退款审核、驳回与结算"
        description="集中处理官方模型充值后的退款单，保留审核备注、外部状态和结算信息，避免财务与运营口径漂移。"
        meta={
          <>
            <StatusBadge tone="warning">{`当前展示 ${refundsQuery.data?.length ?? 0} 条`}</StatusBadge>
            <StatusBadge tone={capabilities.hasCapability('refunds.review') ? 'success' : 'info'}>
              {capabilities.hasCapability('refunds.review') ? '可审核' : '只读巡检'}
            </StatusBadge>
          </>
        }
      />

      <section className="filter-bar-card">
        <div className="filter-bar-card__fields">
          <label className="filter-field">
            <span>用户 ID</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, userId: event.target.value }))} value={filterDraft.userId} />
          </label>
          <label className="filter-field">
            <span>用户关键词</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, keyword: event.target.value }))} value={filterDraft.keyword} />
          </label>
          <label className="filter-field">
            <span>订单号</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, orderId: event.target.value }))} value={filterDraft.orderId} />
          </label>
          <label className="filter-field">
            <span>退款状态</span>
            <select onChange={event => setFilterDraft(current => ({ ...current, status: event.target.value }))} value={filterDraft.status}>
              <option value="">全部</option>
              <option value="pending">待审核</option>
              <option value="approved_pending_payout">待打款</option>
              <option value="refunded">已退款</option>
              <option value="rejected">已驳回</option>
              <option value="refund_failed">退款失败</option>
            </select>
          </label>
        </div>

        <div className="filter-bar-card__actions">
          <button className="button button--ghost" onClick={resetFilters} type="button">
            重置筛选
          </button>
          <button className="button button--primary" onClick={applyFilters} type="button">
            应用筛选
          </button>
        </div>
      </section>

      <InlineStatus tone={statusToneValue}>{statusText}</InlineStatus>

      <div className="panel-grid panel-grid--balanced">
        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>退款请求列表</h2>
              <p>点击任意退款单，在右侧查看完整原因、外部状态和处理表单。</p>
            </div>
          </div>

          <DataTable
            caption="退款请求列表"
            columns={columns}
            emptyMessage="当前筛选条件下未找到退款请求。"
            getRowKey={row => row.id}
            onRowClick={row => {
              setSelectedRefundID(row.id);
              setStatusMessage(`已聚焦退款请求 ${row.id}。`);
            }}
            rows={refundsQuery.data ?? []}
          />
        </section>

        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>退款处理工作台</h2>
              <p>审核备注与结算字段在这里统一维护，所有动作都走显式确认流。</p>
            </div>
          </div>

          {!selectedRefund ? (
            <InlineStatus tone="warning">请选择一条退款请求查看详情。</InlineStatus>
          ) : (
            <div className="form-stack">
              <div className="detail-summary-card">
                <strong>{selectedRefund.id}</strong>
                <p>{selectedRefund.reason || '当前退款请求未填写原因。'}</p>
              </div>

              <dl className="detail-meta-list">
                <div>
                  <dt>用户</dt>
                  <dd>{selectedRefund.username || selectedRefund.user_no || selectedRefund.user_id}</dd>
                </div>
                <div>
                  <dt>订单号</dt>
                  <dd>{selectedRefund.order_id}</dd>
                </div>
                <div>
                  <dt>退款金额</dt>
                  <dd>{formatMoney(selectedRefund.amount_fen)}</dd>
                </div>
                <div>
                  <dt>当前状态</dt>
                  <dd>{statusLabel(selectedRefund.status)}</dd>
                </div>
                <div>
                  <dt>外部状态</dt>
                  <dd>{selectedRefund.external_status || '—'}</dd>
                </div>
                <div>
                  <dt>最近更新</dt>
                  <dd>{formatTimestamp(selectedRefund.updated_unix)}</dd>
                </div>
              </dl>

              <PermissionGate
                capability="refunds.review"
                fallback={<InlineStatus tone="info">当前账号只能查看退款详情，不能执行审核、驳回或结算。</InlineStatus>}
              >
                <div className="form-grid">
                  <label className="filter-field">
                    <span>审核备注</span>
                    <textarea className="textarea-field" onChange={event => setReviewNote(event.target.value)} value={reviewNote} />
                  </label>
                  <label className="filter-field">
                    <span>退款通道</span>
                    <input onChange={event => setRefundProvider(event.target.value)} value={refundProvider} />
                  </label>
                  <label className="filter-field">
                    <span>外部退款单号</span>
                    <input onChange={event => setExternalRefundId(event.target.value)} value={externalRefundId} />
                  </label>
                  <label className="filter-field">
                    <span>外部退款状态</span>
                    <input onChange={event => setExternalStatus(event.target.value)} value={externalStatus} />
                  </label>
                </div>

                <div className="form-actions">
                  <button className="button button--primary" disabled={actionMutation.isPending} onClick={() => void actionMutation.mutateAsync({ action: 'approve', refund: selectedRefund })} type="button">
                    通过退款
                  </button>
                  <button className="button button--ghost" disabled={actionMutation.isPending} onClick={() => void actionMutation.mutateAsync({ action: 'reject', refund: selectedRefund })} type="button">
                    驳回退款
                  </button>
                  <button className="button button--ghost" disabled={actionMutation.isPending} onClick={() => void actionMutation.mutateAsync({ action: 'settle', refund: selectedRefund })} type="button">
                    结算退款
                  </button>
                </div>
              </PermissionGate>
            </div>
          )}
        </section>
      </div>
    </section>
  );
}
