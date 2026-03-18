import { z } from 'zod';

export const adminAuthUserSchema = z.object({
  id: z.string().min(1),
  email: z.string().min(1),
});

export const adminOperatorSchema = z.object({
  user_id: z.string().optional(),
  email: z.string().min(1),
  role: z.string().min(1),
  capabilities: z.array(z.string().min(1)).default([]),
  active: z.boolean(),
  created_unix: z.number().optional(),
  updated_unix: z.number().optional(),
});

export const adminSessionSchema = z.object({
  user: adminAuthUserSchema,
  operator: adminOperatorSchema,
});

export const adminDashboardTotalsSchema = z.object({
  users: z.number().nonnegative(),
  paid_orders: z.number().nonnegative(),
  wallet_balance_fen: z.number(),
  refund_pending: z.number().nonnegative(),
  infringement_pending: z.number().nonnegative(),
});

export const adminDashboardRecentSchema = z.object({
  recharge_fen_7d: z.number(),
  consumption_fen_7d: z.number(),
  new_users_7d: z.number().nonnegative(),
  window_days: z.number().nonnegative().optional(),
});

export const adminDashboardModelStatSchema = z.object({
  model_id: z.string().min(1),
  usage_count: z.number().nonnegative(),
  charged_fen: z.number(),
  prompt_tokens: z.number().nonnegative(),
  completion_tokens: z.number().nonnegative(),
});

export const adminDashboardSchema = z.object({
  totals: adminDashboardTotalsSchema,
  recent: adminDashboardRecentSchema,
  top_models: z.array(adminDashboardModelStatSchema),
  generated_unix: z.number(),
});

export const adminUserSummarySchema = z.object({
  user_id: z.string().min(1),
  user_no: z.number().optional(),
  username: z.string().optional(),
  email: z.string().optional(),
  created_unix: z.number().optional(),
  last_seen_unix: z.number().optional(),
  balance_fen: z.number(),
  currency: z.string(),
  updated_unix: z.number(),
  order_count: z.number().optional(),
  refund_count: z.number().optional(),
  last_order_unix: z.number().optional(),
  last_refund_unix: z.number().optional(),
});

export const walletSummarySchema = z.object({
  user_id: z.string().min(1),
  balance_fen: z.number(),
  currency: z.string(),
  updated_unix: z.number(),
});

export const walletTransactionSchema = z.object({
  id: z.string().min(1),
  user_id: z.string().min(1),
  user_no: z.number().optional(),
  username: z.string().optional(),
  kind: z.string().min(1),
  amount_fen: z.number(),
  description: z.string(),
  reference_type: z.string().optional(),
  reference_id: z.string().optional(),
  pricing_version: z.string().optional(),
  created_unix: z.number(),
});

export const rechargeOrderSchema = z.object({
  id: z.string().min(1),
  user_id: z.string().min(1),
  user_no: z.number().optional(),
  username: z.string().optional(),
  amount_fen: z.number(),
  refunded_fen: z.number().optional(),
  channel: z.string(),
  provider: z.string().optional(),
  status: z.string(),
  pay_url: z.string().optional(),
  external_id: z.string().optional(),
  provider_status: z.string().optional(),
  pricing_version: z.string().optional(),
  agreement_versions: z.array(z.string()).optional(),
  created_unix: z.number(),
  updated_unix: z.number().optional(),
  paid_unix: z.number().optional(),
  last_checked_unix: z.number().optional(),
});

export const refundRequestSchema = z.object({
  id: z.string().min(1),
  user_id: z.string().min(1),
  user_no: z.number().optional(),
  username: z.string().optional(),
  order_id: z.string().min(1),
  amount_fen: z.number(),
  reason: z.string().optional(),
  status: z.string().min(1),
  review_note: z.string().optional(),
  reviewed_by: z.string().optional(),
  refund_provider: z.string().optional(),
  external_refund_id: z.string().optional(),
  external_status: z.string().optional(),
  failure_reason: z.string().optional(),
  created_unix: z.number(),
  updated_unix: z.number(),
  settled_unix: z.number().optional(),
});

export const infringementReportSchema = z.object({
  id: z.string().min(1),
  user_id: z.string().min(1),
  user_no: z.number().optional(),
  username: z.string().optional(),
  subject: z.string().min(1),
  description: z.string().min(1),
  evidence_urls: z.array(z.string()).optional(),
  status: z.string().min(1),
  resolution: z.string().optional(),
  reviewed_by: z.string().optional(),
  created_unix: z.number(),
  updated_unix: z.number(),
});

export const agreementAcceptanceSchema = z.object({
  user_id: z.string().min(1),
  agreement_key: z.string().min(1),
  version: z.string().min(1),
  accepted_unix: z.number(),
  client_version: z.string().optional(),
  remote_addr: z.string().optional(),
  device_summary: z.string().optional(),
  content_checksum: z.string().optional(),
});

export const chatUsageRecordSchema = z.object({
  id: z.string().min(1),
  user_id: z.string().min(1),
  model_id: z.string().min(1),
  pricing_version: z.string().optional(),
  prompt_tokens: z.number().optional(),
  completion_tokens: z.number().optional(),
  charged_fen: z.number(),
  fallback_applied: z.boolean().optional(),
  request_kind: z.string().optional(),
  created_unix: z.number(),
  agreement_versions: z.array(z.string()).optional(),
});

export const adminAuditLogSchema = z.object({
  id: z.string().min(1),
  actor_user_id: z.string().optional(),
  actor_email: z.string().optional(),
  action: z.string().min(1),
  target_type: z.string().optional(),
  target_id: z.string().optional(),
  risk_level: z.string().optional(),
  detail: z.string().optional(),
  created_unix: z.number(),
});

export const adminUserOverviewSchema = z.object({
  user: adminUserSummarySchema,
  wallet: walletSummarySchema,
  recent_orders: z.array(rechargeOrderSchema),
  recent_transactions: z.array(walletTransactionSchema),
  agreements: z.array(agreementAcceptanceSchema),
  recent_usage: z.array(chatUsageRecordSchema),
  pending_refund_count: z.number().nonnegative(),
  pending_infringement_count: z.number().nonnegative(),
});

export const officialModelSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1),
  description: z.string().optional(),
  enabled: z.boolean(),
  pricing_version: z.string().optional(),
});

export const pricingRuleSchema = z.object({
  model_id: z.string().min(1),
  version: z.string().optional(),
  effective_from_unix: z.number().optional(),
  input_price_micros_per_1k: z.number(),
  output_price_micros_per_1k: z.number(),
  fallback_price_fen: z.number(),
});

export const agreementDocumentSchema = z.object({
  key: z.string().min(1),
  version: z.string().min(1),
  title: z.string().min(1),
  content: z.string().optional(),
  url: z.string().optional(),
  effective_from_unix: z.number().optional(),
});

export const routeModelConfigSchema = z
  .object({
    model_name: z.string().min(1),
    model: z.string().min(1),
    api_base: z.string().optional(),
    api_key: z.string().optional(),
    proxy: z.string().optional(),
    auth_method: z.string().optional(),
    connect_mode: z.string().optional(),
    workspace: z.string().optional(),
    rpm: z.number().optional(),
    max_tokens_field: z.string().optional(),
    request_timeout: z.number().optional(),
    thinking_level: z.string().optional(),
  })
  .catchall(z.unknown());

export const officialRouteSchema = z.object({
  public_model_id: z.string().min(1),
  model_config: routeModelConfigSchema,
});

export const runtimeConfigStateSchema = z.object({
  official_routes: z.array(officialRouteSchema),
  official_models: z.array(officialModelSchema),
  pricing_rules: z.array(pricingRuleSchema),
  agreements: z.array(agreementDocumentSchema),
});

export const systemNoticeSchema = z.object({
  id: z.string().min(1),
  title: z.string().min(1),
  body: z.string().min(1),
  severity: z.string().optional(),
  enabled: z.boolean(),
  updated_unix: z.number().optional(),
});

export const riskRuleSchema = z.object({
  key: z.string().min(1),
  name: z.string().min(1),
  description: z.string().optional(),
  enabled: z.boolean(),
  updated_unix: z.number().optional(),
});

export const dataRetentionPolicySchema = z.object({
  data_domain: z.string().min(1),
  retention_days: z.number(),
  purge_mode: z.string().optional(),
  description: z.string().optional(),
  enabled: z.boolean(),
  updated_unix: z.number().optional(),
});
