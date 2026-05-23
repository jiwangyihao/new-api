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

export const subscriptionPlanSchema = z.object({
  id: z.number(),
  title: z.string(),
  subtitle: z.string().optional(),
  price_amount: z.number(),
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
  monthly_token_limit: z.number().optional(),
  concurrency_limit: z.number().optional(),
  is_trial: z.boolean().optional(),
  invite_trial: z.boolean().optional(),
  public_visible: z.boolean().optional(),
  trial_duration_hours: z.number().optional(),
  reward_eligible: z.boolean().optional(),
  business_code: z.string().optional(),
})

export type SubscriptionPlan = z.infer<typeof subscriptionPlanSchema>

export interface PlanRecord {
  plan: SubscriptionPlan
}

export interface PublicSubscriptionPlan {
  id: number
  title: string
  subtitle: string
  price_amount: number
  currency: string
  duration_unit: SubscriptionPlan['duration_unit']
  duration_value: number
  custom_seconds: number
  monthly_token_limit: number
  concurrency_limit: number
  public_visible: boolean
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
  grant_reason: z.string().optional(),
  grant_source_user_id: z.number().optional(),
  effective_end_time: z.number().optional(),
  is_active_selected: z.boolean().optional(),
  can_reset_quota: z.boolean().optional(),
  source_label: z.string().optional(),
})

export type UserSubscription = z.infer<typeof userSubscriptionSchema>

export interface UserSubscriptionRecord {
  subscription: UserSubscription
  plan?: SubscriptionPlan
  plan_title?: string
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface PlanPayload {
  plan: Partial<SubscriptionPlan>
}

export interface SubscriptionPayRequest {
  plan_id: number
  payment_method?: string
}

export interface SubscriptionBalancePayRequest {
  plan_id: number
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

export type SubscriptionBalancePayResponse = ApiResponse<SubscriptionOrderRecord>

export interface SubscriptionPayResponse {
  success: boolean
  message?: string
  data?: {
    pay_link?: string
    checkout_url?: string
  }
  url?: string
}

export interface CreateUserSubscriptionRequest {
  plan_id: number
}

// ============================================================================
// Self Subscription Data (user-facing)
// ============================================================================

export interface SelfSubscriptionSummary {
  active_subscription_id?: number
  active_count: number
  subscription_id?: number
  plan_id?: number
  primary_plan_title?: string
  token_limit: number
  token_used: number
  token_remaining: number
  token_unlimited: boolean
  concurrency_limit: number
  next_reset_time?: number
  end_time?: number
}

export interface SelfSubscriptionData {
  billing_preference: string
  subscriptions: UserSubscriptionRecord[]
  all_subscriptions: UserSubscriptionRecord[]
  summary: SelfSubscriptionSummary
  active_subscription_id?: number
}

// ============================================================================
// Dialog Types
// ============================================================================

export type SubscriptionsDialogType = 'create' | 'update' | 'toggle-status'
