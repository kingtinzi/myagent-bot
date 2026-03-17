import {
  agreementAcceptanceSchema,
  agreementDocumentSchema,
  adminAuditLogSchema,
  adminDashboardSchema,
  adminOperatorSchema,
  adminSessionSchema,
  adminUserOverviewSchema,
  adminUserSummarySchema,
  chatUsageRecordSchema,
  dataRetentionPolicySchema,
  officialModelSchema,
  officialRouteSchema,
  pricingRuleSchema,
  rechargeOrderSchema,
  riskRuleSchema,
  systemNoticeSchema,
  runtimeConfigStateSchema,
  walletSummarySchema,
  walletTransactionSchema,
} from '../schemas/admin';
import type {
  AdminAuditLogsQuery,
  AdminDashboard,
  AdminDashboardQuery,
  AdminLoginInput,
  AdminSession,
  AdminUserOverview,
  AdminUsersQuery,
  DataRetentionPolicy,
  RiskRule,
  RevisionedResponse,
  RuntimeConfigState,
  SystemNotice,
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

  async listAuditLogs(query: AdminAuditLogsQuery = {}) {
    const response = await requestJSON<unknown[]>(
      withSearchParams('/admin/audit-logs', {
        action: query.action,
        target_type: query.targetType,
        target_id: query.targetId,
        actor_user_id: query.actorUserId,
        risk_level: query.riskLevel,
        since_unix: query.sinceUnix,
        until_unix: query.untilUnix,
        limit: query.limit,
        offset: query.offset,
      }),
    );

    return adminAuditLogSchema.array().parse(response.data);
  },

  buildAuditLogsExportURL(query: AdminAuditLogsQuery = {}) {
    return withSearchParams('/admin/audit-logs', {
      action: query.action,
      target_type: query.targetType,
      target_id: query.targetId,
      actor_user_id: query.actorUserId,
      risk_level: query.riskLevel,
      since_unix: query.sinceUnix,
      until_unix: query.untilUnix,
      limit: query.limit,
      offset: query.offset,
      format: 'csv',
    });
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

  async listUserOrders(userID: string) {
    const response = await requestJSON<unknown[]>(`/admin/users/${encodeURIComponent(userID)}/orders`);
    return rechargeOrderSchema.array().parse(response.data);
  },

  async listUserAgreements(userID: string) {
    const response = await requestJSON<unknown[]>(`/admin/users/${encodeURIComponent(userID)}/agreements`);
    return agreementAcceptanceSchema.array().parse(response.data);
  },

  async listUserUsage(userID: string) {
    const response = await requestJSON<unknown[]>(`/admin/users/${encodeURIComponent(userID)}/usage`);
    return chatUsageRecordSchema.array().parse(response.data);
  },

  async listWalletAdjustments(query: { userId?: string; keyword?: string }) {
    const response = await requestJSON<unknown[]>(
      withSearchParams('/admin/wallet-adjustments', {
        user_id: query.userId,
        keyword: query.keyword,
        limit: 20,
      }),
    );
    return walletTransactionSchema.array().parse(response.data);
  },

  async createManualRecharge(payload: { user_id: string; amount_fen: number; description?: string; request_id: string }) {
    const response = await requestJSON<unknown>('/admin/manual-recharges', {
      method: 'POST',
      body: payload,
    });
    return walletSummarySchema.parse(response.data);
  },

  async createWalletAdjustment(payload: { user_id: string; amount_fen: number; description?: string; request_id: string }) {
    const response = await requestJSON<unknown>('/admin/wallet-adjustments', {
      method: 'POST',
      body: payload,
    });
    return walletSummarySchema.parse(response.data);
  },

  async listOrders(query: { userId?: string; keyword?: string; status?: string }) {
    const response = await requestJSON<unknown[]>(
      withSearchParams('/admin/orders', {
        user_id: query.userId,
        keyword: query.keyword,
        status: query.status,
        limit: 20,
      }),
    );
    return rechargeOrderSchema.array().parse(response.data);
  },

  async reconcileOrder(orderId: string) {
    const response = await requestJSON<{ changed: boolean; order: unknown }>(`/admin/orders/${encodeURIComponent(orderId)}/reconcile`, {
      method: 'POST',
    });

    return {
      changed: Boolean(response.data.changed),
      order: rechargeOrderSchema.parse(response.data.order),
    };
  },

  async listOperators() {
    const response = await requestJSON<unknown[]>('/admin/operators');
    return adminOperatorSchema.array().parse(response.data);
  },

  async saveOperator(email: string, payload: { role: string; active: boolean }) {
    const response = await requestJSON<unknown>(`/admin/operators/${encodeURIComponent(email)}`, {
      method: 'PUT',
      body: payload,
    });
    return adminOperatorSchema.parse(response.data);
  },

  async getOfficialModels() {
    return parseRevisioned(requestJSON<unknown[]>('/admin/models'), data => officialModelSchema.array().parse(data));
  },

  async saveOfficialModels(payload: unknown, revision: string) {
    return parseRevisioned(
      requestJSON<unknown[]>('/admin/models', {
        method: 'PUT',
        body: payload as Record<string, unknown>[] | Record<string, unknown>,
        headers: {
          'If-Match': revision,
        },
      }),
      data => officialModelSchema.array().parse(data),
    );
  },

  async getOfficialRoutes() {
    return parseRevisioned(requestJSON<unknown[]>('/admin/model-routes'), data => officialRouteSchema.array().parse(data));
  },

  async saveOfficialRoutes(payload: unknown, revision: string) {
    return parseRevisioned(
      requestJSON<unknown[]>('/admin/model-routes', {
        method: 'PUT',
        body: payload as Record<string, unknown>[] | Record<string, unknown>,
        headers: {
          'If-Match': revision,
        },
      }),
      data => officialRouteSchema.array().parse(data),
    );
  },

  async getPricingRules() {
    return parseRevisioned(requestJSON<unknown[]>('/admin/pricing-rules'), data => pricingRuleSchema.array().parse(data));
  },

  async savePricingRules(payload: unknown, revision: string) {
    return parseRevisioned(
      requestJSON<unknown[]>('/admin/pricing-rules', {
        method: 'PUT',
        body: payload as Record<string, unknown>[] | Record<string, unknown>,
        headers: {
          'If-Match': revision,
        },
      }),
      data => pricingRuleSchema.array().parse(data),
    );
  },

  async getAgreementVersions() {
    return parseRevisioned(requestJSON<unknown[]>('/admin/agreement-versions'), data => agreementDocumentSchema.array().parse(data));
  },

  async saveAgreementVersions(payload: unknown, revision: string) {
    return parseRevisioned(
      requestJSON<unknown[]>('/admin/agreement-versions', {
        method: 'PUT',
        body: payload as Record<string, unknown>[] | Record<string, unknown>,
        headers: {
          'If-Match': revision,
        },
      }),
      data => agreementDocumentSchema.array().parse(data),
    );
  },

  async getSystemNotices() {
    return parseRevisioned(requestJSON<unknown[]>('/admin/system-notices'), data => systemNoticeSchema.array().parse(data));
  },

  async saveSystemNotices(payload: SystemNotice[], revision: string) {
    return parseRevisioned(
      requestJSON<unknown[]>('/admin/system-notices', {
        method: 'PUT',
        body: payload,
        headers: {
          'If-Match': revision,
        },
      }),
      data => systemNoticeSchema.array().parse(data),
    );
  },

  async getRiskRules() {
    return parseRevisioned(requestJSON<unknown[]>('/admin/risk-rules'), data => riskRuleSchema.array().parse(data));
  },

  async saveRiskRules(payload: RiskRule[], revision: string) {
    return parseRevisioned(
      requestJSON<unknown[]>('/admin/risk-rules', {
        method: 'PUT',
        body: payload,
        headers: {
          'If-Match': revision,
        },
      }),
      data => riskRuleSchema.array().parse(data),
    );
  },

  async getDataRetentionPolicies() {
    return parseRevisioned(requestJSON<unknown[]>('/admin/data-retention-policies'), data => dataRetentionPolicySchema.array().parse(data));
  },

  async saveDataRetentionPolicies(payload: DataRetentionPolicy[], revision: string) {
    return parseRevisioned(
      requestJSON<unknown[]>('/admin/data-retention-policies', {
        method: 'PUT',
        body: payload,
        headers: {
          'If-Match': revision,
        },
      }),
      data => dataRetentionPolicySchema.array().parse(data),
    );
  },

  async getWallet(): Promise<WalletSummary> {
    const response = await requestJSON<unknown>('/wallet');
    return walletSummarySchema.parse(response.data);
  },

  async listWalletTransactions() {
    const response = await requestJSON<unknown[]>('/wallet/transactions');
    return walletTransactionSchema.array().parse(response.data);
  },

  async getRuntimeConfig(): Promise<RevisionedResponse<RuntimeConfigState>> {
    return parseRevisioned(requestJSON<unknown>('/admin/runtime-config'), data => runtimeConfigStateSchema.parse(data));
  },

  async saveRuntimeConfig(payload: RuntimeConfigState, revision: string) {
    return parseRevisioned(
      requestJSON<unknown>('/admin/runtime-config', {
        method: 'PUT',
        body: payload as Record<string, unknown>,
        headers: {
          'If-Match': revision,
        },
      }),
      data => runtimeConfigStateSchema.parse(data),
    );
  },
};
