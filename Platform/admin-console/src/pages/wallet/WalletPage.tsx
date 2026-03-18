import { useMemo, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { DataTable } from '../../components/data/DataTable';
import { FilterBar } from '../../components/data/FilterBar';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { PageHeader } from '../../components/layout/PageHeader';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import { useCapabilities } from '../../hooks/useCapabilities';
import { ManualRechargePanel } from '../../features/wallet-mutation/ManualRechargePanel';
import { WalletAdjustmentPanel } from '../../features/wallet-mutation/WalletAdjustmentPanel';
import type { AdminUserSummary, WalletTransaction } from '../../services/contracts';

type WalletLocationState = {
  userId?: string;
};

function buildRequestId(prefix: string, userId: string) {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `${prefix}-${userId}-${crypto.randomUUID()}`;
  }
  return `${prefix}-${userId}-${Date.now()}`;
}

function normalizeErrorMessage(error: unknown, fallback: string) {
  if (error instanceof Error) {
    const message = error.message.trim();
    if (message) {
      return message;
    }
  }
  return fallback;
}

function formatUserTarget(user: AdminUserSummary) {
  const username = user.username?.trim();
  const userNo = typeof user.user_no === 'number' ? user.user_no : undefined;
  if (username && userNo) {
    return `${username}（#${userNo}）`;
  }
  if (username) {
    return username;
  }
  if (userNo) {
    return `#${userNo}`;
  }
  return user.user_id;
}

async function resolveUserTarget(userKey: string) {
  const normalized = userKey.trim();
  const byID = await adminApi.listUsers({ userId: normalized, limit: 1 });
  if (byID.length === 1) {
    return byID[0];
  }

  const matches = await adminApi.listUsers({ keyword: normalized, limit: 5 });
  if (matches.length === 1) {
    return matches[0];
  }
  if (matches.length === 0) {
    throw new Error('未找到匹配的用户，请输入用户名、编号、邮箱，或从用户列表进入。');
  }
  throw new Error('匹配到多个用户，请输入更精确的用户名、编号或邮箱。');
}

export function WalletPage() {
  const location = useLocation();
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const capabilities = useCapabilities();
  const locationState = (location.state ?? {}) as WalletLocationState;
  const [draftKeyword, setDraftKeyword] = useState(locationState.userId ?? '');
  const [keyword, setKeyword] = useState(locationState.userId ?? '');
  const [statusMessage, setStatusMessage] = useState('钱包页已就绪。');
  const canWriteWallet = capabilities.canWrite('wallet');

  const adjustmentsQuery = useQuery({
    queryKey: ['admin', 'wallet-adjustments', keyword],
    queryFn: () =>
      adminApi.listWalletAdjustments({
        keyword: keyword.trim() || undefined,
        userId: keyword.trim() || undefined,
      }),
    retry: false,
  });

  const manualRechargeMutation = useMutation({
    mutationFn: async (payload: { userId: string; amountFen: number; description: string }) => {
      const targetUser = await resolveUserTarget(payload.userId);
      const confirmed = await confirmAction({
        title: '管理员手动充值',
        message: `即将为用户 ${formatUserTarget(targetUser)}充值 ${payload.amountFen} 分。`,
        hint: payload.description || '未填写说明',
        confirmLabel: '确认充值',
        tone: 'warning',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.createManualRecharge({
        user_id: targetUser.user_id,
        amount_fen: payload.amountFen,
        description: payload.description,
        request_id: buildRequestId('manual-recharge', targetUser.user_id),
      });
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('管理员手动充值已取消。');
        return;
      }
      setStatusMessage(`充值完成，最新余额 ${result.currency} ${(result.balance_fen / 100).toFixed(2)}。`);
      void queryClient.invalidateQueries({ queryKey: ['admin', 'wallet-adjustments'] });
    },
    onError: error => {
      setStatusMessage(normalizeErrorMessage(error, '手动充值失败，请稍后重试。'));
    },
  });

  const walletAdjustmentMutation = useMutation({
    mutationFn: async (payload: { userId: string; amountFen: number; description: string }) => {
      const targetUser = await resolveUserTarget(payload.userId);
      const confirmed = await confirmAction({
        title: '钱包调账',
        message: `即将为用户 ${formatUserTarget(targetUser)}调账 ${payload.amountFen} 分。`,
        hint: payload.description || '未填写说明',
        confirmLabel: '确认调账',
        tone: 'danger',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.createWalletAdjustment({
        user_id: targetUser.user_id,
        amount_fen: payload.amountFen,
        description: payload.description,
        request_id: buildRequestId('wallet-adjustment', targetUser.user_id),
      });
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('钱包调账已取消。');
        return;
      }
      setStatusMessage(`调账完成，最新余额 ${result.currency} ${(result.balance_fen / 100).toFixed(2)}。`);
      void queryClient.invalidateQueries({ queryKey: ['admin', 'wallet-adjustments'] });
    },
    onError: error => {
      setStatusMessage(normalizeErrorMessage(error, '钱包调账失败，请稍后重试。'));
    },
  });

  const columns = useMemo(
    () => [
      { key: 'user', header: '用户 ID', cell: (row: WalletTransaction) => row.user_id },
      { key: 'kind', header: '类型', cell: (row: WalletTransaction) => row.kind },
      { key: 'amount', header: '金额（分）', cell: (row: WalletTransaction) => row.amount_fen.toLocaleString('zh-CN') },
      { key: 'description', header: '说明', cell: (row: WalletTransaction) => row.description },
      { key: 'reference', header: '参考类型', cell: (row: WalletTransaction) => row.reference_type || '—' },
    ],
    [],
  );

  if (!capabilities.canAccessModule('wallet')) {
    return <InlineStatus tone="warning">当前管理员没有查看钱包与调账模块的权限。</InlineStatus>;
  }

  return (
    <section className="page-stack">
      <PageHeader eyebrow="财务中台" title="钱包与手动充值" description="查询钱包流水、手动充值和调账，所有写操作都走统一确认流。" />

      <FilterBar
        onSubmit={event => {
          event.preventDefault();
          setKeyword(draftKeyword.trim());
        }}
        actions={
          <button className="button button--primary" type="submit">
            查询钱包
          </button>
        }
      >
        <label className="filter-field">
          <span>用户关键字</span>
          <input onChange={event => setDraftKeyword(event.target.value)} placeholder="用户 ID / 用户名 / 编号" value={draftKeyword} />
        </label>
      </FilterBar>

      <InlineStatus tone={adjustmentsQuery.isFetching ? 'info' : 'success'}>
        {adjustmentsQuery.isFetching ? '正在加载钱包流水…' : statusMessage}
      </InlineStatus>

      {canWriteWallet ? (
        <div className="panel-grid panel-grid--balanced">
          <ManualRechargePanel onSubmit={payload => manualRechargeMutation.mutateAsync(payload)} presetUserId={keyword} />
          <WalletAdjustmentPanel onSubmit={payload => walletAdjustmentMutation.mutateAsync(payload)} presetUserId={keyword} />
        </div>
      ) : (
        <InlineStatus tone="warning">当前账号仅有钱包查看权限，不能执行手动充值或调账。</InlineStatus>
      )}

      <DataTable
        caption="钱包流水"
        columns={columns}
        emptyMessage="当前没有符合条件的钱包流水。"
        getRowKey={row => row.id}
        rows={adjustmentsQuery.data ?? []}
      />
    </section>
  );
}
