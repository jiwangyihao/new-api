/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { z } from 'zod'

// ============================================================================
// Subscription Plan Schema & Types
// ============================================================================

const channelEquivalentBaseSchema = z.object({
  channel_type: z.number(),
  channel_type_name: z.string(),
  channel_type_label_key: z.string().optional(),
  variant_count: z.number(),
})

const usageTokenPlanSingleEquivalentSchema = channelEquivalentBaseSchema.extend(
  {
    kind: z.literal('usage_tokens'),
    value_type: z.literal('single'),
    multiplier: z.number(),
    equivalent_token_limit: z.number(),
  }
)

const usageTokenPlanRangeEquivalentSchema = channelEquivalentBaseSchema.extend({
  kind: z.literal('usage_tokens'),
  value_type: z.literal('range'),
  min_multiplier: z.number(),
  max_multiplier: z.number(),
  equivalent_token_limit_min: z.number(),
  equivalent_token_limit_max: z.number(),
})

const fixedRequestPlanSingleEquivalentSchema =
  channelEquivalentBaseSchema.extend({
    kind: z.literal('fixed_request'),
    value_type: z.literal('single'),
    fixed_request_credits: z.number(),
    equivalent_request_limit: z.number(),
  })

const fixedRequestPlanRangeEquivalentSchema =
  channelEquivalentBaseSchema.extend({
    kind: z.literal('fixed_request'),
    value_type: z.literal('range'),
    fixed_request_credits_min: z.number(),
    fixed_request_credits_max: z.number(),
    equivalent_request_limit_min: z.number(),
    equivalent_request_limit_max: z.number(),
  })

const unlimitedPlanEquivalentSchema = channelEquivalentBaseSchema.extend({
  kind: z.literal('unlimited'),
  value_type: z.literal('unlimited'),
  credit_unlimited: z.literal(true).optional(),
  token_unlimited: z.literal(true).optional(),
})

const planChannelCreditEquivalentSchema = z.union([
  usageTokenPlanSingleEquivalentSchema,
  usageTokenPlanRangeEquivalentSchema,
  fixedRequestPlanSingleEquivalentSchema,
  fixedRequestPlanRangeEquivalentSchema,
  unlimitedPlanEquivalentSchema,
])

const usageTokenSubscriptionSingleEquivalentSchema =
  channelEquivalentBaseSchema.extend({
    kind: z.literal('usage_tokens'),
    value_type: z.literal('single'),
    multiplier: z.number(),
    equivalent_token_limit: z.number(),
    equivalent_token_remaining: z.number(),
  })

const usageTokenSubscriptionRangeEquivalentSchema =
  channelEquivalentBaseSchema.extend({
    kind: z.literal('usage_tokens'),
    value_type: z.literal('range'),
    min_multiplier: z.number(),
    max_multiplier: z.number(),
    equivalent_token_limit_min: z.number(),
    equivalent_token_limit_max: z.number(),
    equivalent_token_remaining_min: z.number(),
    equivalent_token_remaining_max: z.number(),
  })

const fixedRequestSubscriptionSingleEquivalentSchema =
  channelEquivalentBaseSchema.extend({
    kind: z.literal('fixed_request'),
    value_type: z.literal('single'),
    fixed_request_credits: z.number(),
    equivalent_request_limit: z.number(),
    equivalent_request_remaining: z.number(),
  })

const fixedRequestSubscriptionRangeEquivalentSchema =
  channelEquivalentBaseSchema.extend({
    kind: z.literal('fixed_request'),
    value_type: z.literal('range'),
    fixed_request_credits_min: z.number(),
    fixed_request_credits_max: z.number(),
    equivalent_request_limit_min: z.number(),
    equivalent_request_limit_max: z.number(),
    equivalent_request_remaining_min: z.number(),
    equivalent_request_remaining_max: z.number(),
  })

const unlimitedSubscriptionEquivalentSchema =
  channelEquivalentBaseSchema.extend({
    kind: z.literal('unlimited'),
    value_type: z.literal('unlimited'),
    credit_unlimited: z.literal(true).optional(),
    token_unlimited: z.literal(true).optional(),
  })

const _subscriptionChannelCreditEquivalentSchema = z.union([
  usageTokenSubscriptionSingleEquivalentSchema,
  usageTokenSubscriptionRangeEquivalentSchema,
  fixedRequestSubscriptionSingleEquivalentSchema,
  fixedRequestSubscriptionRangeEquivalentSchema,
  unlimitedSubscriptionEquivalentSchema,
])

const legacyPlanChannelTokenEquivalentSchema = z.discriminatedUnion('kind', [
  channelEquivalentBaseSchema.extend({
    kind: z.literal('single'),
    multiplier: z.number(),
    equivalent_token_limit: z.number(),
  }),
  channelEquivalentBaseSchema.extend({
    kind: z.literal('range'),
    min_multiplier: z.number(),
    max_multiplier: z.number(),
    equivalent_token_limit_min: z.number(),
    equivalent_token_limit_max: z.number(),
  }),
  channelEquivalentBaseSchema.extend({
    kind: z.literal('unlimited'),
    token_unlimited: z.literal(true),
  }),
])

const _legacySubscriptionChannelTokenEquivalentSchema = z.discriminatedUnion(
  'kind',
  [
    channelEquivalentBaseSchema.extend({
      kind: z.literal('single'),
      multiplier: z.number(),
      equivalent_token_limit: z.number(),
      equivalent_token_remaining: z.number(),
    }),
    channelEquivalentBaseSchema.extend({
      kind: z.literal('range'),
      min_multiplier: z.number(),
      max_multiplier: z.number(),
      equivalent_token_limit_min: z.number(),
      equivalent_token_limit_max: z.number(),
      equivalent_token_remaining_min: z.number(),
      equivalent_token_remaining_max: z.number(),
    }),
    channelEquivalentBaseSchema.extend({
      kind: z.literal('unlimited'),
      token_unlimited: z.literal(true),
    }),
  ]
)

const planChannelTokenEquivalentSchema = z.union([
  legacyPlanChannelTokenEquivalentSchema,
  planChannelCreditEquivalentSchema,
])

export type PlanChannelCreditEquivalent = z.infer<
  typeof planChannelCreditEquivalentSchema
>

export type SubscriptionChannelCreditEquivalent = z.infer<
  typeof _subscriptionChannelCreditEquivalentSchema
>

export type PlanChannelTokenEquivalent = z.infer<
  typeof planChannelTokenEquivalentSchema
>

export type SubscriptionChannelTokenEquivalent =
  | z.infer<typeof _legacySubscriptionChannelTokenEquivalentSchema>
  | SubscriptionChannelCreditEquivalent

export const subscriptionPlanSchema = z.object({
  id: z.number(),
  title: z.string(),
  subtitle: z.string().optional(),
  price_amount: z.number(),
  price_amount_micros: z.string().nullable().optional(),
  valuation_currency: z.string().nullable().optional(),
  currency: z.string().default('USD'),
  duration_unit: z.enum(['year', 'month', 'day', 'hour', 'custom']),
  duration_value: z.number(),
  custom_seconds: z.number().optional(),
  quota_reset_period: z.enum(['never', 'daily', 'weekly', 'monthly', 'custom']),
  quota_reset_custom_seconds: z.number().optional(),
  enabled: z.boolean(),
  sort_order: z.number(),
  max_purchase_per_user: z.number(),
  total_amount: z.number(),
  stripe_price_id: z.string().optional(),
  creem_product_id: z.string().optional(),
  kyren_product_id: z.string().optional(),
  monthly_token_limit: z.number().optional(),
  concurrency_limit: z.number().optional(),
  queue_capacity: z.number().optional(),
  gpt_abuse_warning_limit: z.number().optional(),
  is_trial: z.boolean().optional(),
  invite_trial: z.boolean().optional(),
  public_visible: z.boolean().optional(),
  trial_duration_hours: z.number().optional(),
  reward_eligible: z.boolean().optional(),
  business_code: z.string().optional(),
  entitlement_type: z.enum(['timed', 'credit_balance']).default('timed'),
  credit_balance_configured: z.boolean().optional(),
  credit_balance_purchase_enabled: z.boolean().optional(),
  credit_balance_redemption_enabled: z.boolean().optional(),
  credit_balance_conversion_enabled: z.boolean().optional(),
  unlimited_purchase_enabled: z.boolean().optional(),
  timed_conversion_enabled: z.boolean().optional(),
  channel_credit_equivalents: z
    .array(planChannelCreditEquivalentSchema)
    .default([]),
  channel_token_equivalents: z
    .array(planChannelTokenEquivalentSchema)
    .default([]),
})

export type SubscriptionPlan = Omit<
  z.infer<typeof subscriptionPlanSchema>,
  | 'channel_credit_equivalents'
  | 'channel_token_equivalents'
  | 'entitlement_type'
> & {
  entitlement_type?: 'timed' | 'credit_balance'
  channel_credit_equivalents?: PlanChannelCreditEquivalent[]
  channel_token_equivalents?: PlanChannelTokenEquivalent[]
}

export interface PlanRecord {
  plan: SubscriptionPlan
  existing_timed_entitlement_count?: number
}

export interface PublicSubscriptionPlan {
  id: number
  title: string
  subtitle: string
  price_amount: number
  price_amount_micros?: string | null
  currency: string
  kyren_product_id?: string
  duration_unit: SubscriptionPlan['duration_unit']
  duration_value: number
  custom_seconds: number
  monthly_token_limit: number
  concurrency_limit: number
  queue_capacity: number
  gpt_abuse_warning_limit: number
  public_visible: boolean
  enabled?: boolean
  is_trial?: boolean
  max_purchase_per_user?: number
  channel_credit_equivalents?: PlanChannelCreditEquivalent[]
  channel_token_equivalents?: PlanChannelTokenEquivalent[]
}

export interface PublicPlanRecord {
  plan: PublicSubscriptionPlan
}

// ============================================================================
// User Subscription Schema & Types
// ============================================================================

export const userSubscriptionSchema = z.object({
  id: z.number(),
  user_id: z.number(),
  plan_id: z.number(),
  status: z.string(),
  source: z.string().optional(),
  start_time: z.number(),
  end_time: z.number(),
  amount_total: z.number(),
  amount_used: z.number(),
  next_reset_time: z.number().optional(),
  token_used: z.number().optional(),
  token_limit: z.number().optional(),
  concurrency_limit: z.number().optional(),
  queue_capacity: z.number().optional(),
  grant_reason: z.string().optional(),
  grant_source_user_id: z.number().optional(),
  entitlement_type: z.enum(['timed', 'credit_balance']).default('timed'),
  effective_end_time: z.number().optional(),
  is_active_selected: z.boolean().optional(),
  can_reset_quota: z.boolean().optional(),
  source_label: z.string().optional(),
  converted_at: z.number().optional(),
})

export type UserSubscription = Omit<
  z.infer<typeof userSubscriptionSchema>,
  'entitlement_type'
> & {
  entitlement_type?: 'timed' | 'credit_balance'
}

export interface SubscriptionConversionAudit {
  conversion_id: string
  source_subscription_id: string
  target_subscription_id: string
  source_status_before: string
  source_status_after: string
  target_status: string
  converted_at: string
}

export interface UserSubscriptionRecord {
  subscription: UserSubscription
  plan_title?: string
  plan?: SubscriptionPlan
  conversion_audit?: SubscriptionConversionAudit
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  code?: string
  data?: T
}

export type CodexProMode = 'all' | 'flexible' | 'off'
export type ApiKeyCodexProMode = 'inherit' | CodexProMode
export type CodexProUnavailableReason =
  | ''
  | 'wallet_only'
  | 'trial_subscription'
  | 'reward_subscription'
  | 'no_paid_subscription'
  | 'features_hidden'
export interface UpdateCodexProModeRequest {
  mode: CodexProMode
}

export interface UpdateCodexProModeResponse {
  codex_pro_mode: CodexProMode
  codex_pro_eligible: boolean
  codex_pro_unavailable_reason: CodexProUnavailableReason
  codex_pro_features_hidden?: boolean
}

export interface PlanPayload {
  plan: Omit<Partial<SubscriptionPlan>, 'price_amount'> & {
    price_amount?: string
    price_amount_micros?: string
  }
  risk_confirmed?: boolean
  risk_reason?: string
}

export interface CreditBalancePlanUpdateRequest {
  concurrency_limit: number
  queue_capacity: number
  business_code: string
  configured: boolean
  valuation_currency: 'CNY' | 'USD'
  purchase_enabled: boolean
  redemption_enabled: boolean
  conversion_enabled: boolean
}

export interface SubscriptionPayRequest {
  plan_id: number
  purchase_mode: SubscriptionPurchaseMode
  payment_method?: string
  retry_trade_no?: string
}

export type SubscriptionKyrenProductSyncMode =
  | 'create_or_update'
  | 'create_new'
  | 'update_existing'

export interface SubscriptionKyrenProductStatus {
  bound?: boolean
  missing?: boolean
  archived?: boolean
  product_id?: string
  status?: string
  price?: string
  currency?: string
  price_matches?: boolean
  currency_matches?: boolean
}

export interface SubscriptionKyrenProductSyncStatus extends SubscriptionKyrenProductStatus {
  synced?: boolean
  local_error?: string
}

export type SubscriptionKyrenProductResponse =
  ApiResponse<SubscriptionKyrenProductStatus>

export type SubscriptionKyrenProductSyncResponse =
  ApiResponse<SubscriptionKyrenProductSyncStatus>

export type SubscriptionKyrenPayResponse = SubscriptionPayResponse

export type SubscriptionPurchaseMode = 'timed' | 'credit_balance'

export interface SubscriptionBalancePayRequest {
  plan_id: number
  purchase_mode: SubscriptionPurchaseMode
  idempotency_key: string
}

export interface SubscriptionOrderRecord {
  id: number
  user_id: number
  plan_id: number
  money: number
  trade_no: string
  payment_method: string
  payment_provider: string
  status: string
  create_time: number
  complete_time: number
  provider_payload: string
}

export interface CreditBalanceGrantResult {
  user_subscription_id: number
  plan_id: number
  gross_credit: number
  debt_offset: number
  available_credit: number
  settlement_debt: number
  balance_before: number
  balance_after: number
  active: boolean
  ledger_id: number
  status: 'available' | 'exhausted' | 'debt'
}

export interface SubscriptionOrderStatus {
  trade_no: string
  plan_id: number
  payment_provider: string
  payment_method: string
  purchase_mode: SubscriptionPurchaseMode
  status: 'pending' | 'success' | 'failed' | 'expired' | string
  create_time: number
  complete_time: number
  checkout_url?: string
  credit_balance?: CreditBalanceGrantResult
}

export type CreditBalanceAdjustmentOperation = 'increase' | 'decrease'

export interface CreditBalanceAdjustmentRequest {
  operation: CreditBalanceAdjustmentOperation
  amount: string
  plan_id?: number
  idempotency_key: string
  reason: string
}

export interface CreditBalanceAdjustmentPreviewRequest {
  operation: CreditBalanceAdjustmentOperation
  amount: string
  plan_id?: number
}

export interface CreditBalanceAdjustmentAuthoritativeResult {
  plan_id: number
  gross_credit: number
  net_credit: number
  gross_amount_micros: string
  net_amount_micros: string
  valuation_currency: string
  source_currency: string
  confidence: string
  fx_rate_numerator: string
  fx_rate_denominator: string
  fx_captured_at: number
  fx_direction: string
  rule_version: number
  state_version_after: number
  consumed_available_credit: number
  debt_formed: number
  removed_exact_cost_micros: string
  removed_estimated_cost_micros: string
  removed_unknown_credit: number
  operation: string
  terminal_state: string
  debt_offset: number
  available_credit: number
  settlement_debt: number
  balance_before: number
  balance_after: number
  replayed: boolean
  preview: boolean
}

export interface CreditBalanceAdjustmentPreviewResult extends CreditBalanceAdjustmentAuthoritativeResult {
  credit_balance: CreditBalanceGrantResult
}

export interface CreditBalanceAdjustmentResult extends CreditBalanceAdjustmentAuthoritativeResult {
  adjustment: {
    id: number
    idempotency_key: string
    user_id: number
    operation: CreditBalanceAdjustmentOperation
    amount: number
    operator_user_id: number
    reason: string
    ledger_id: number
    created_at: number
  }
  credit_balance: CreditBalanceGrantResult
}

export interface SubscriptionOrderRecoveryPreview {
  order_id: number
  user_id: number
  username: string
  plan_id: number
  plan_title: string
  trade_no: string
  money: number
  amount_cents: number
  currency: string
  payment_provider: string
  payment_method: string
  purchase_mode: SubscriptionPurchaseMode
  status: string
  complete_time: number
  gross_credit: number
}

export interface SubscriptionOrderRecoveryRequest {
  recovery_type: 'refund' | 'chargeback'
  reason: string
}

export interface SubscriptionOrderRecoveryResult {
  order_id: number
  trade_no: string
  status: 'refunded' | 'chargeback'
  recovery_type: 'refund' | 'chargeback'
  gross_credit: number
  consumed_available_credit: number
  removed_exact_cost_micros: string
  removed_estimated_cost_micros: string
  removed_unknown_credit: number
  valuation_currency: string
  rule_version: number
  state_version_after: number
  operation: string
  terminal_state: string
  debt_formed: number
  available_credit: number
  settlement_debt: number
  balance_before: number
  balance_after: number
  ledger_id: number
  replayed: boolean
}

export interface CreditBalanceLedgerFilters {
  source_type?: string
  type?: string
  start_time?: number
  end_time?: number
}

export interface CreditBalanceLedgerEntry {
  id: number
  user_id: number
  user_subscription_id: number
  type: string
  idempotency_key: string
  source_type: string
  source_id: number
  net_credit: number
  operation: string
  terminal_state: string
  plan_id: number
  gross_credit: number
  debt_offset: number
  debt_formed?: number
  consumed_available_credit: number
  settlement_debt_formed: number
  removed_exact_cost_micros: string
  removed_estimated_cost_micros: string
  removed_unknown_credit: number
  available_credit_before?: number
  settlement_debt_before?: number
  balance_before: number
  balance_after: number
  available_credit_after: number
  settlement_debt_after: number
  valuation_currency: string
  valuation_rule_version: number
  valuation_state_version_after: number
  operator_user_id?: number
  reason: string
  source_snapshot?: string
  created_at: number
  payment_provider?: string
  payment_method?: string
  purchase_mode?: SubscriptionPurchaseMode
}

export type SubscriptionBalancePayResponse = ApiResponse<{
  order: SubscriptionOrderRecord
  purchase_mode: SubscriptionPurchaseMode
  credit_balance?: CreditBalanceGrantResult
}>

export interface SubscriptionPayResponse {
  success: boolean
  message?: string
  data?: {
    pay_link?: string
    checkout_url?: string
    order_id?: string
  }
  url?: string
  order_id?: string
}

export interface CreateUserSubscriptionRequest {
  plan_id: number
  idempotency_key: string
  reason: string
  source_price_micros: string
  source_currency: string
}

// ============================================================================
// Self Subscription Data (user-facing)
// ============================================================================

export type SubscriptionBillingStrategy =
  | 'single_active'
  | 'active_fallback'
  | 'timed_first'

export interface SelfSubscriptionSummary {
  active_subscription_id?: number
  billing_strategy?: SubscriptionBillingStrategy
  billing_candidate_subscription_ids?: number[]
  active_count: number
  subscription_id?: number
  plan_id?: number
  primary_plan_title?: string
  token_limit: number
  token_used: number
  token_remaining: number
  token_unlimited: boolean
  concurrency_limit: number
  queue_capacity?: number
  gpt_abuse_warning_limit: number
  gpt_abuse_warning_count: number
  gpt_abuse_warning_remaining: number
  gpt_abuse_suspended_until?: number
  gpt_abuse_limit_enabled: boolean
  next_reset_time?: number
  end_time?: number
  channel_credit_equivalents?: SubscriptionChannelCreditEquivalent[]
  channel_token_equivalents?: SubscriptionChannelTokenEquivalent[]
}

export interface SelfSubscriptionData {
  active_subscription_id?: number
  billing_preference?: string
  billing_strategy?: SubscriptionBillingStrategy
  billing_candidate_subscription_ids?: number[]
  last_subscription_purchase_mode?: SubscriptionPurchaseMode
  credit_balance?: CreditBalanceGrantResult | null
  credit_balance_ledger?: CreditBalanceLedgerEntry[]
  credit_balance_purchase_enabled?: boolean
  credit_balance_plan?: {
    concurrency_limit?: number
    queue_capacity?: number
  } | null
  codex_pro_mode?: CodexProMode
  codex_pro_eligible?: boolean
  codex_pro_unavailable_reason?: CodexProUnavailableReason
  codex_pro_features_hidden?: boolean
  subscriptions: UserSubscriptionRecord[]
  all_subscriptions: UserSubscriptionRecord[]
  summary: SelfSubscriptionSummary
}

// ============================================================================
// Dialog Types
// ============================================================================

export type SubscriptionsDialogType = 'create' | 'update' | 'toggle-status'
