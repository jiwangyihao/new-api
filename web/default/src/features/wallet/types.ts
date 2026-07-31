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
// ============================================================================
// Wallet Type Definitions
// ============================================================================

/**
 * Generic API response
 */
export interface ApiResponse<T = unknown> {
  success?: boolean
  message?: string
  data?: T
  code?: string
}

/**
 * Standard API response types
 */
export type TopupInfoResponse = ApiResponse<TopupInfo>
export type RedemptionResponse = ApiResponse<number> & {
  result?: RedemptionResult
}
export type AmountResponse = ApiResponse<string>
export type PaymentResponse = ApiResponse<Record<string, unknown>> & {
  url?: string
}
export type StripePaymentResponse = ApiResponse<{ pay_link: string }>
export type AffiliateCodeResponse = ApiResponse<string>
export type AffiliateTransferResponse = ApiResponse
export type InvitationEntitlementResponse = ApiResponse<InvitationEntitlement>
export type CreemPaymentResponse = ApiResponse<{ checkout_url: string }>
export type KyrenPaymentResponse = ApiResponse<{
  checkout_url?: string
  pay_link?: string
  url?: string
}>
export type WaffoPaymentResponse = ApiResponse<
  { payment_url?: string } | string
>
export type WaffoPancakePaymentResponse = ApiResponse<
  | {
      checkout_url?: string
      session_id?: string
      expires_at?: number | string
      order_id?: string
    }
  | string
>

export interface PageParams {
  page: number
  page_size: number
}

export interface PageEnvelope<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface InvitationCommissionContact {
  type: 'wechat' | 'telegram' | 'email' | 'other'
  value: string
}

export interface InvitationCommissionSummary {
  reward_mode: 'subscription' | 'commission'
  has_commission_account: boolean
  can_transfer: boolean
  can_request_withdrawal: boolean
  direct_invite_count: number
  qualified_paid_invite_count: number
  account: {
    available_cents: number
    pending_cents: number
    withdrawn_cents: number
    transferred_cents: number
  }
  setting: {
    enabled: boolean
    minimum_withdraw_cents: number
    minimum_transfer_cents: number
    rate_bps: number
  }
}

export interface InvitationCommissionRecord {
  id: number
  event_id: number
  invitee_id: number
  source_type: string
  source_id: number
  source_trade_no: string
  source_amount_cents: number
  source_currency: string
  commission_rate_bps: number
  commission_cents: number
  status: 'available' | 'skipped' | 'cancelled' | 'unrecoverable'
  reason: string
  created_at: number
  available_at: number
  cancelled_at: number
  reversal_status?: 'recovered' | 'unrecoverable'
  recovered_cents?: number
  unrecovered_cents?: number
  reversal_reason?: string
  reversed_at?: number
}

export interface InvitationCommissionTransferResult {
  available_cents: number
  transferred_cents: number
  user_quota: number
}

export interface InvitationCommissionWithdrawalPayload {
  amount_cents: number
  contact: InvitationCommissionContact
  remark?: string
}

export interface InvitationCommissionWithdrawal {
  id: number
  amount_cents: number
  status: 'pending' | 'completed' | 'rejected'
  method: 'manual'
  contact: InvitationCommissionContact
  user_remark: string
  admin_remark: string
  reviewer_id: number
  completed_by: number
  completed_at: number
  reviewed_at: number
  created_at: number
  updated_at: number
}

/**
 * Creem product configuration
 */
export interface CreemProduct {
  /** Product display name */
  name: string
  /** Creem product ID */
  productId: string
  /** Product price */
  price: number
  /** Quota amount to credit */
  quota: number
  /** Currency (USD or EUR) */
  currency: 'USD' | 'EUR'
}

/**
 * Creem payment request
 */
export interface CreemPaymentRequest {
  /** Creem product ID */
  product_id: string
  /** Payment method identifier */
  payment_method: 'creem'
}

/**
 * Kyren top-up product shown to users. Uses local product IDs only.
 */
export interface KyrenTopUpProduct {
  /** Local top-up product ID */
  id: string
  /** Product display name */
  name: string
  /** Optional product description */
  description?: string
  /** CNY amount as a fixed decimal string */
  amount: string
  /** Product currency */
  currency: 'CNY'
  /** Quota amount to credit */
  quota: number
  /** Whether product is available */
  enabled?: boolean
}

/**
 * Kyren payment request
 */
export interface KyrenPaymentRequest {
  /** Local Kyren top-up product ID */
  product_id: string
}

/**
 * Payment method configuration
 */
export interface PaymentMethod {
  /** Display name of payment method */
  name: string
  /** Payment method type identifier */
  type: string
  /** Optional color for UI display */
  color?: string
  /** Minimum topup amount for this payment method */
  min_topup?: number
  /** Optional icon URL provided by backend (preferred over built-in icons) */
  icon?: string
}

/**
 * Waffo payment method configuration
 */
export interface WaffoPayMethod {
  /** Display name of payment method */
  name: string
  /** Optional icon path */
  icon?: string
  /** Waffo pay method type */
  payMethodType?: string
  /** Waffo pay method name */
  payMethodName?: string
}

/**
 * Topup configuration information
 */
export interface TopupInfo {
  /** Whether online topup is enabled */
  enable_online_topup: boolean
  /** Whether Stripe topup is enabled */
  enable_stripe_topup: boolean
  /** Available payment methods */
  pay_methods: PaymentMethod[]
  /** Minimum topup amount for online topup */
  min_topup: number
  /** Minimum topup amount for Stripe */
  stripe_min_topup: number
  /** Preset amount options */
  amount_options: number[]
  /** Discount rates by amount */
  discount: Record<number, number>
  /** Optional topup link for purchasing codes */
  topup_link?: string
  /** Whether Creem topup is enabled */
  enable_creem_topup?: boolean
  /** Available Creem products */
  creem_products?: CreemProduct[]
  /** Whether Kyren wallet topup is enabled */
  enable_kyren_topup?: boolean
  /** Whether Kyren subscription payment is enabled */
  enable_kyren_subscription?: boolean
  /** Available Kyren local top-up products */
  kyren_topup_products?: KyrenTopUpProduct[]
  /** Whether Waffo topup is enabled */
  enable_waffo_topup?: boolean
  /** Available Waffo payment methods */
  waffo_pay_methods?: WaffoPayMethod[]
  /** Minimum topup amount for Waffo */
  waffo_min_topup?: number
  /** Whether Waffo Pancake topup is enabled */
  enable_waffo_pancake_topup?: boolean
  /** Minimum topup amount for Waffo Pancake */
  waffo_pancake_min_topup?: number
}

/**
 * Preset amount option with optional discount
 */
export interface PresetAmount {
  /** Preset amount value */
  value: number
  /** Optional discount rate (0-1) */
  discount?: number
}

/**
 * Redemption code request
 */
export type RedemptionMode = 'timed' | 'credit_balance'

export interface RedemptionCreditBalanceResult {
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

export interface RedemptionRequest {
  /** Redemption code key */
  key: string
  /** Explicit benefit mode for subscription codes */
  redemption_mode?: RedemptionMode
}

export interface RedemptionResult {
  type: 'wallet' | 'subscription'
  quota: number
  redemption_id?: number
  redemption_mode?: RedemptionMode
  fulfillment_subscription_id?: number
  replayed?: boolean
  credit_balance?: RedemptionCreditBalanceResult
  plan?: {
    id: number
    title: string
  }
}

/**
 * Payment request parameters
 */
export interface PaymentRequest {
  /** Topup amount */
  amount: number
  /** Payment method identifier */
  payment_method: string
}

/**
 * Waffo payment request parameters
 */
export interface WaffoPaymentRequest {
  /** Topup amount */
  amount: number
  /** Optional server-side Waffo payment method index */
  pay_method_index?: number
}

/**
 * Waffo Pancake payment request parameters
 */
export interface WaffoPancakePaymentRequest {
  /** Topup amount */
  amount: number
}

/**
 * Amount calculation request
 */
export interface AmountRequest {
  /** Topup amount to calculate */
  amount: number
}

/**
 * Affiliate quota transfer request
 */
export interface AffiliateTransferRequest {
  /** Quota amount to transfer */
  quota: number
}

export interface InvitationEntitlement {
  direct_invite_count: number
  qualified_active_count: number
  reward_month: string
  entitled: boolean
  entitlement_end_time: number
  reward_subscription_id?: number
  reward_plan_id?: number
  reward_plan_title?: string
  reward_plan_business_code?: string
  reward_tier_rank?: number
  reward_tier_qualified_count?: number
  downgrade_reward_plan_id?: number
  downgrade_reward_plan_title?: string
  downgrade_reward_plan_business_code?: string
  downgrade_reward_tier_rank?: number
  downgrade_reward_tier_qualified_count?: number
  downgrade_entitlement_end_time?: number
}

/**
 * User wallet data
 */
export interface UserWalletData {
  /** User ID */
  id: number
  /** Username */
  username: string
  /** Current quota balance */
  quota: number
  /** Total used quota */
  used_quota: number
  /** Total request count */
  request_count: number
  /** Affiliate quota (pending rewards) */
  aff_quota: number
  /** Total affiliate quota earned (historical) */
  aff_history_quota: number
  /** Number of successful affiliate invites */
  aff_count: number
}

/**
 * Topup record status
 */
export type TopupStatus = 'success' | 'pending' | 'expired'

/**
 * Topup billing record
 */
export interface TopupRecord {
  /** Record ID */
  id: number
  /** User ID */
  user_id: number
  /** Topup amount (quota) */
  amount: number
  /** Payment amount (actual money paid) */
  money: number
  /** Account balance credited by this order, in CNY cents */
  credited_balance_cents?: number
  /** Server-provided display string for credited account balance */
  credited_balance_display?: string
  /** Whether credited balance uses account-balance cents semantics */
  is_account_balance_cents?: boolean
  /** Unit used for the credited amount display/audit fields */
  amount_unit?: string
  /** Trade/order number */
  trade_no: string
  /** Payment method type */
  payment_method: string
  /** Creation timestamp */
  create_time: number
  /** Completion timestamp */
  complete_time?: number
  /** Payment status */
  status: TopupStatus
}

/**
 * Billing history response
 */
export interface BillingHistoryResponse {
  items: TopupRecord[]
  total: number
}

/**
 * Complete order request (admin only)
 */
export interface CompleteOrderRequest {
  trade_no: string
}
