import { useQuery } from '@tanstack/react-query';

import { adminApi } from '../../services/adminApi';

export const adminDashboardQueryKey = (sinceDays: number) => ['admin', 'dashboard', sinceDays] as const;

export function useAdminDashboard(sinceDays: number) {
  return useQuery({
    queryKey: adminDashboardQueryKey(sinceDays),
    queryFn: () => adminApi.getDashboard({ sinceDays }),
    retry: false,
  });
}
