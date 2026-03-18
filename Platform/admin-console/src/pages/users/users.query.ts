import { useQuery } from '@tanstack/react-query';

import { adminApi } from '../../services/adminApi';

export const adminUsersQueryKey = (keyword: string) => ['admin', 'users', keyword] as const;
export const adminUserOverviewQueryKey = (userId: string) => ['admin', 'user-overview', userId] as const;
export const adminUserWalletTransactionsQueryKey = (userId: string) => ['admin', 'user-wallet-transactions', userId] as const;
export const adminUserOrdersQueryKey = (userId: string) => ['admin', 'user-orders', userId] as const;
export const adminUserAgreementsQueryKey = (userId: string) => ['admin', 'user-agreements', userId] as const;
export const adminUserUsageQueryKey = (userId: string) => ['admin', 'user-usage', userId] as const;

export function useAdminUsers(keyword: string) {
  return useQuery({
    queryKey: adminUsersQueryKey(keyword),
    queryFn: () =>
      adminApi.listUsers({
        keyword: keyword.trim() || undefined,
        limit: 20,
      }),
    retry: false,
  });
}

export function useAdminUserOverview(userId: string) {
  return useQuery({
    queryKey: adminUserOverviewQueryKey(userId),
    queryFn: () => adminApi.getUserOverview(userId),
    enabled: Boolean(userId),
    retry: false,
  });
}

export function useAdminUserWalletTransactions(userId: string, enabled: boolean) {
  return useQuery({
    queryKey: adminUserWalletTransactionsQueryKey(userId),
    queryFn: () => adminApi.listUserWalletTransactions(userId),
    enabled: enabled && Boolean(userId),
    retry: false,
  });
}

export function useAdminUserOrders(userId: string, enabled: boolean) {
  return useQuery({
    queryKey: adminUserOrdersQueryKey(userId),
    queryFn: () => adminApi.listUserOrders(userId),
    enabled: enabled && Boolean(userId),
    retry: false,
  });
}

export function useAdminUserAgreements(userId: string, enabled: boolean) {
  return useQuery({
    queryKey: adminUserAgreementsQueryKey(userId),
    queryFn: () => adminApi.listUserAgreements(userId),
    enabled: enabled && Boolean(userId),
    retry: false,
  });
}

export function useAdminUserUsage(userId: string, enabled: boolean) {
  return useQuery({
    queryKey: adminUserUsageQueryKey(userId),
    queryFn: () => adminApi.listUserUsage(userId),
    enabled: enabled && Boolean(userId),
    retry: false,
  });
}
