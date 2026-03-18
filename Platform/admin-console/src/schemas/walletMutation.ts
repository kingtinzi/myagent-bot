import { z } from 'zod';

export const manualRechargeFormSchema = z.object({
  userId: z.string().trim().min(1, '请填写用户标识。'),
  amountFen: z.coerce.number().int().positive('充值金额必须大于 0。'),
  description: z.string().trim().max(200).optional().default(''),
});

export const walletAdjustmentFormSchema = z.object({
  userId: z.string().trim().min(1, '请填写用户标识。'),
  amountFen: z.coerce.number().int().refine(value => value !== 0, '调账金额不能为 0。'),
  description: z.string().trim().max(200).optional().default(''),
});
