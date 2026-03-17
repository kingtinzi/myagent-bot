import type { z } from 'zod';

import type {
  agreementAcceptanceSchema,
  adminAuthUserSchema,
  adminDashboardSchema,
  agreementDocumentSchema,
  adminOperatorSchema,
  adminSessionSchema,
  adminUserOverviewSchema,
  adminUserSummarySchema,
  adminAuditLogSchema,
  chatUsageRecordSchema,
  dataRetentionPolicySchema,
  infringementReportSchema,
  officialModelSchema,
  officialRouteSchema,
  pricingRuleSchema,
  rechargeOrderSchema,
  refundRequestSchema,
  routeModelConfigSchema,
  riskRuleSchema,
  systemNoticeSchema,
  runtimeConfigStateSchema,
  walletSummarySchema,
  walletTransactionSchema,
} from '../schemas/admin';

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger';

export type AdminAuthUser = z.infer<typeof adminAuthUserSchema>;
export type AdminOperator = z.infer<typeof adminOperatorSchema>;
export type AdminSession = z.infer<typeof adminSessionSchema>;
export type AdminDashboard = z.infer<typeof adminDashboardSchema>;
export type AdminUserSummary = z.infer<typeof adminUserSummarySchema>;
export type AdminUserOverview = z.infer<typeof adminUserOverviewSchema>;
export type AdminAuditLog = z.infer<typeof adminAuditLogSchema>;
export type WalletSummary = z.infer<typeof walletSummarySchema>;
export type WalletTransaction = z.infer<typeof walletTransactionSchema>;
export type RechargeOrder = z.infer<typeof rechargeOrderSchema>;
export type RefundRequest = z.infer<typeof refundRequestSchema>;
export type InfringementReport = z.infer<typeof infringementReportSchema>;
export type AgreementAcceptance = z.infer<typeof agreementAcceptanceSchema>;
export type ChatUsageRecord = z.infer<typeof chatUsageRecordSchema>;
export type OfficialModel = z.infer<typeof officialModelSchema>;
export type PricingRule = z.infer<typeof pricingRuleSchema>;
export type AgreementDocument = z.infer<typeof agreementDocumentSchema>;
export type RouteModelConfig = z.infer<typeof routeModelConfigSchema>;
export type OfficialRoute = z.infer<typeof officialRouteSchema>;
export type RuntimeConfigState = z.infer<typeof runtimeConfigStateSchema>;
export type SystemNotice = z.infer<typeof systemNoticeSchema>;
export type RiskRule = z.infer<typeof riskRuleSchema>;
export type DataRetentionPolicy = z.infer<typeof dataRetentionPolicySchema>;

export type AdminOrderReconcileResult = {
  changed: boolean;
  order: RechargeOrder;
};

export type AdminSessionStatus = 'idle' | 'loading' | 'authenticated' | 'anonymous' | 'error';

export type RevisionedResponse<T> = {
  data: T;
  revision: string | null;
  status: number;
};

export type AdminLoginInput = {
  email: string;
  password: string;
};

export type AdminDashboardQuery = {
  sinceDays?: number;
};

export type AdminAuditLogsQuery = {
  action?: string;
  targetType?: string;
  targetId?: string;
  actorUserId?: string;
  riskLevel?: string;
  sinceUnix?: number;
  untilUnix?: number;
  limit?: number;
  offset?: number;
};

export type AdminUsersQuery = {
  userId?: string;
  email?: string;
  keyword?: string;
  limit?: number;
  offset?: number;
};

export type AdminWalletMutationInput = {
  user_id: string;
  amount_fen: number;
  description?: string;
  request_id?: string;
};

export type AdminRefundsQuery = {
  userId?: string;
  keyword?: string;
  orderId?: string;
  status?: string;
  limit?: number;
  offset?: number;
};

export type AdminRefundDecisionInput = {
  review_note?: string;
  refund_provider?: string;
  external_refund_id?: string;
  external_status?: string;
};

export type AdminInfringementQuery = {
  userId?: string;
  keyword?: string;
  status?: string;
  reviewedBy?: string;
  limit?: number;
  offset?: number;
};

export type AdminInfringementUpdateInput = {
  status: string;
  resolution?: string;
};

export type ConfirmActionConfig = {
  title: string;
  message: string;
  hint?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: StatusTone;
  requireText?: string;
};
