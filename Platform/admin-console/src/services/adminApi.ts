import {
  adminDashboardSchema,
  adminSessionSchema,
  adminUserOverviewSchema,
  adminUserSummarySchema,
  walletSummarySchema,
  walletTransactionSchema,
} from '../schemas/admin';
import type {
  AdminDashboard,
  AdminDashboardQuery,
  AdminLoginInput,
  AdminSession,
  AdminUserOverview,
  AdminUsersQuery,
  RevisionedResponse,
  WalletSummary,
  WalletTransaction,
} from './contracts';
import { requestJSON } from './http';

function withSearchParams(path: string, params: Record<string, string | number | undefined>) {
  const search = new URLSearchParams();

  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') {
      return;
    }
    search.set(key, String(value));
  });

  const query = search.toString();
  return query ? `${path}?${query}` : path;
}

async function parseRevisioned<T>(request: Promise<{ data: unknown; status: number; revision: string | null }>, parser: (value: unknown) => T): Promise<RevisionedResponse<T>> {
  const response = await request;

  return {
    data: parser(response.data),
    revision: response.revision,
    status: response.status,
  };
}

export const adminApi = {
  async getSession(): Promise<AdminSession> {
    const response = await requestJSON<unknown>('/admin/session');
    return adminSessionSchema.parse(response.data);
  },

  async login(input: AdminLoginInput): Promise<AdminSession> {
    const response = await requestJSON<unknown>('/admin/session/login', {
      method: 'POST',
      body: input,
    });

    return adminSessionSchema.parse(response.data);
  },

  async logout(): Promise<void> {
    await requestJSON<null>('/admin/session/logout', {
      method: 'POST',
    });
  },

  async getDashboard(query: AdminDashboardQuery = {}): Promise<AdminDashboard> {
    const response = await requestJSON<unknown>(
      withSearchParams('/admin/dashboard', {
        since_days: query.sinceDays,
      }),
    );

    return adminDashboardSchema.parse(response.data);
  },

  async listUsers(query: AdminUsersQuery = {}) {
    const response = await requestJSON<unknown[]>(
      withSearchParams('/admin/users', {
        user_id: query.userId,
        email: query.email,
        keyword: query.keyword,
        limit: query.limit,
        offset: query.offset,
      }),
    );

    return adminUserSummarySchema.array().parse(response.data);
  },

  async getUserOverview(userID: string): Promise<AdminUserOverview> {
    const response = await requestJSON<unknown>(`/admin/users/${encodeURIComponent(userID)}/overview`);
    return adminUserOverviewSchema.parse(response.data);
  },

  async listUserWalletTransactions(userID: string) {
    const response = await requestJSON<unknown[]>(`/admin/users/${encodeURIComponent(userID)}/wallet-transactions`);
    return walletTransactionSchema.array().parse(response.data);
  },

  async getWallet(): Promise<WalletSummary> {
    const response = await requestJSON<unknown>('/wallet');
    return walletSummarySchema.parse(response.data);
  },

  async listWalletTransactions() {
    const response = await requestJSON<unknown[]>('/wallet/transactions');
    return walletTransactionSchema.array().parse(response.data);
  },

  async getRuntimeConfig(): Promise<RevisionedResponse<unknown>> {
    return parseRevisioned(requestJSON<unknown>('/admin/runtime-config'), data => data);
  },

  async saveRuntimeConfig(payload: unknown, revision: string) {
    return parseRevisioned(
      requestJSON<unknown>('/admin/runtime-config', {
        method: 'PUT',
        body: payload as Record<string, unknown>,
        headers: {
          'If-Match': revision,
        },
      }),
      data => data,
    );
  },
};
