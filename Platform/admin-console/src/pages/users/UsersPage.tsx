import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { DataTable } from '../../components/data/DataTable';
import { FilterBar } from '../../components/data/FilterBar';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { UserIdentityCell } from '../../components/identity/UserIdentityCell';
import { PageHeader } from '../../components/layout/PageHeader';
import { useCapabilities } from '../../hooks/useCapabilities';
import type { AdminUserSummary } from '../../services/contracts';
import { UserDetailDrawer } from '../../features/user-detail/UserDetailDrawer';
import { useAdminUsers } from './users.query';

export function UsersPage() {
  const navigate = useNavigate();
  const capabilities = useCapabilities();
  const [draftKeyword, setDraftKeyword] = useState('');
  const [keyword, setKeyword] = useState('');
  const [selectedUserId, setSelectedUserId] = useState('');
  const usersQuery = useAdminUsers(keyword);

  const columns = useMemo(
    () => [
      {
        key: 'identity',
        header: '用户',
        cell: (row: AdminUserSummary) => (
          <UserIdentityCell email={row.email} userNo={row.user_no} username={row.username} />
        ),
      },
      {
        key: 'balance',
        header: '余额',
        cell: (row: AdminUserSummary) => `${row.currency} ${(row.balance_fen / 100).toFixed(2)}`,
      },
      {
        key: 'orders',
        header: '订单',
        cell: (row: AdminUserSummary) => (row.order_count ?? 0).toLocaleString('zh-CN'),
      },
      {
        key: 'updated',
        header: '更新时间',
        cell: (row: AdminUserSummary) => new Date(row.updated_unix * 1000).toLocaleString('zh-CN'),
      },
    ],
    [],
  );

  if (!capabilities.canAccessModule('users')) {
    return <InlineStatus tone="warning">当前管理员没有查看用户中心的权限。</InlineStatus>;
  }

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="账号中心"
        title="用户与账号"
        description="统一查看账号、钱包、协议、订单与模型用量。优先以用户名 / 用户编号 / 邮箱识别用户。"
      />

      <FilterBar
        onSubmit={event => {
          event.preventDefault();
          setKeyword(draftKeyword.trim());
        }}
        actions={
          <>
            <button className="button button--ghost" onClick={() => { setDraftKeyword(''); setKeyword(''); }} type="button">
              重置
            </button>
            <button className="button button--primary" type="submit">
              搜索用户
            </button>
          </>
        }
      >
        <label className="filter-field">
          <span>关键字</span>
          <input
            onChange={event => setDraftKeyword(event.target.value)}
            placeholder="用户名 / 用户编号 / 邮箱"
            value={draftKeyword}
          />
        </label>
      </FilterBar>

      {usersQuery.isError ? (
        <InlineStatus tone="warning">
          {usersQuery.error instanceof Error ? usersQuery.error.message : '加载用户列表失败。'}
        </InlineStatus>
      ) : (
        <InlineStatus tone={usersQuery.isFetching ? 'info' : 'success'}>
          {usersQuery.isFetching ? '正在加载用户列表…' : '点击任意用户行可展开详情抽屉。'}
        </InlineStatus>
      )}

      <DataTable
        caption="用户列表"
        columns={columns}
        emptyMessage="当前没有匹配的用户。"
        getRowKey={row => row.user_id}
        onRowClick={row => setSelectedUserId(row.user_id)}
        rows={usersQuery.data ?? []}
      />

      <UserDetailDrawer
        key={selectedUserId || 'user-detail-drawer'}
        onClose={() => setSelectedUserId('')}
        onOpenOrders={userId => navigate('/orders', { state: { userId } })}
        onOpenWallet={userId => navigate('/wallet', { state: { userId } })}
        open={Boolean(selectedUserId)}
        userId={selectedUserId}
      />
    </section>
  );
}
