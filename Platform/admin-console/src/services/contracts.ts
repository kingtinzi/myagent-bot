import type { z } from 'zod';

import type {
  agreementAcceptanceSchema,
  adminAuthUserSchema,
  adminDashboardSchema,
  adminOperatorSchema,
  adminSessionSchema,
  adminUserOverviewSchema,
  adminUserSummarySchema,
  chatUsageRecordSchema,
  rechargeOrderSchema,
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
export type WalletSummary = z.infer<typeof walletSummarySchema>;
export type WalletTransaction = z.infer<typeof walletTransactionSchema>;
export type RechargeOrder = z.infer<typeof rechargeOrderSchema>;
export type AgreementAcceptance = z.infer<typeof agreementAcceptanceSchema>;
export type ChatUsageRecord = z.infer<typeof chatUsageRecordSchema>;

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

export type ConfirmActionConfig = {
  title: string;
  message: string;
  hint?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: StatusTone;
  requireText?: string;
};
