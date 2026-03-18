import { useMemo, useState } from 'react';

import { DetailDrawer } from '../../components/data/DetailDrawer';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { UserIdentityCell } from '../../components/identity/UserIdentityCell';
import { useCapabilities } from '../../hooks/useCapabilities';
import {
  useAdminUserAgreements,
  useAdminUserOrders,
  useAdminUserOverview,
  useAdminUserUsage,
  useAdminUserWalletTransactions,
} from '../../pages/users/users.query';
import { UserAgreementsTab } from './UserAgreementsTab';
import { UserOrdersTab } from './UserOrdersTab';
import { UserOverviewTab } from './UserOverviewTab';
import { UserUsageTab } from './UserUsageTab';
import { UserWalletTab } from './UserWalletTab';

type UserDetailDrawerProps = {
  onClose: () => void;
  onOpenOrders: (userId: string) => void;
  onOpenWallet: (userId: string) => void;
  open: boolean;
  userId: string;
};

type TabKey = 'overview' | 'wallet' | 'orders' | 'agreements' | 'usage';

const tabOrder: { key: TabKey; label: string; capability: string }[] = [
  { key: 'overview', label: '概览', capability: 'users.read' },
  { key: 'wallet', label: '钱包', capability: 'wallet.read' },
  { key: 'orders', label: '订单', capability: 'orders.read' },
  { key: 'agreements', label: '协议', capability: 'agreements.read' },
  { key: 'usage', label: '用量', capability: 'usage.read' },
];

export function UserDetailDrawer({ onClose, onOpenOrders, onOpenWallet, open, userId }: UserDetailDrawerProps) {
  const [activeTab, setActiveTab] = useState<TabKey>('overview');
  const capabilities = useCapabilities();
  const overviewQuery = useAdminUserOverview(userId);
  const walletQuery = useAdminUserWalletTransactions(userId, open && activeTab === 'wallet' && capabilities.hasCapability('wallet.read'));
  const ordersQuery = useAdminUserOrders(userId, open && activeTab === 'orders' && capabilities.hasCapability('orders.read'));
  const agreementsQuery = useAdminUserAgreements(userId, open && activeTab === 'agreements' && capabilities.hasCapability('agreements.read'));
  const usageQuery = useAdminUserUsage(userId, open && activeTab === 'usage' && capabilities.hasCapability('usage.read'));

  const visibleTabs = useMemo(
    () => tabOrder.filter(tab => tab.key === 'overview' || capabilities.hasCapability(tab.capability)),
    [capabilities],
  );

  const overview = overviewQuery.data;

  const title = overview?.user.username?.trim() || overview?.user.email?.trim() || '用户详情';

  return (
    <DetailDrawer
      actions={
        overview ? (
          <>
            <button className="button button--ghost" onClick={() => onOpenOrders(userId)} type="button">
              查看订单
            </button>
            <button className="button button--primary" onClick={() => onOpenWallet(userId)} type="button">
              手动充值
            </button>
          </>
        ) : undefined
      }
      description={overview ? `用户编号 ${overview.user.user_no ? `#${overview.user.user_no}` : '未分配'}` : '正在加载用户详情'}
      onClose={onClose}
      open={open}
      title={title}
    >
      {overview ? (
        <>
          <div className="detail-drawer__identity">
            <UserIdentityCell email={overview.user.email} userNo={overview.user.user_no} username={overview.user.username} />
          </div>

          <div aria-label="用户详情标签" className="detail-tabs" role="tablist">
            {visibleTabs.map(tab => (
              <button
                aria-selected={activeTab === tab.key}
                className={['detail-tabs__button', activeTab === tab.key ? 'is-active' : ''].filter(Boolean).join(' ')}
                key={tab.key}
                onClick={() => setActiveTab(tab.key)}
                role="tab"
                type="button"
              >
                {tab.label}
              </button>
            ))}
          </div>

          {activeTab === 'overview' ? <UserOverviewTab overview={overview} /> : null}
          {activeTab === 'wallet' ? <UserWalletTab transactions={walletQuery.data ?? []} wallet={overview.wallet} /> : null}
          {activeTab === 'orders' ? <UserOrdersTab orders={ordersQuery.data ?? []} /> : null}
          {activeTab === 'agreements' ? <UserAgreementsTab agreements={agreementsQuery.data ?? []} /> : null}
          {activeTab === 'usage' ? <UserUsageTab items={usageQuery.data ?? []} /> : null}
        </>
      ) : overviewQuery.isError ? (
        <InlineStatus tone="warning">
          {overviewQuery.error instanceof Error ? overviewQuery.error.message : '加载用户详情失败。'}
        </InlineStatus>
      ) : (
        <InlineStatus tone="info">正在加载用户详情…</InlineStatus>
      )}
    </DetailDrawer>
  );
}
