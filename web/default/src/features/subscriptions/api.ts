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
import { api } from '@/lib/api'
import type {
  ApiResponse,
  PlanRecord,
  PublicPlanRecord,
  PlanPayload,
  UserSubscriptionRecord,
  CreateUserSubscriptionRequest,
  SubscriptionPlan,
  CreditBalancePlanUpdateRequest,
  SubscriptionPayResponse,
  SubscriptionPayRequest,
  SelfSubscriptionData,
  SubscriptionBalancePayRequest,
  SubscriptionBalancePayResponse,
  SubscriptionKyrenPayResponse,
  SubscriptionKyrenProductResponse,
  SubscriptionKyrenProductSyncResponse,
  SubscriptionKyrenProductSyncMode,
  UpdateCodexProModeRequest,
  UpdateCodexProModeResponse,
  SubscriptionOrderStatus,
  SubscriptionBillingStrategy,
} from './types'

export interface SetActiveSubscriptionRequest {
  subscription_id: number
}

export interface UpdateSubscriptionBillingStrategyRequest {
  billing_strategy: SubscriptionBillingStrategy
}

// ============================================================================
// Admin Plan Management
// ============================================================================

export async function getAdminPlans(): Promise<ApiResponse<PlanRecord[]>> {
  const res = await api.get('/api/subscription/admin/plans')
  return res.data
}

export async function getCreditBalancePlan(): Promise<
  ApiResponse<SubscriptionPlan>
> {
  const res = await api.get('/api/subscription/admin/credit-balance-plan')
  return res.data
}

export async function updateCreditBalancePlan(
  data: CreditBalancePlanUpdateRequest
): Promise<ApiResponse<SubscriptionPlan>> {
  const res = await api.put('/api/subscription/admin/credit-balance-plan', data)
  return res.data
}

export async function createPlan(
  data: PlanPayload
): Promise<ApiResponse<PlanRecord>> {
  const res = await api.post('/api/subscription/admin/plans', data)
  return res.data
}

export async function updatePlan(
  id: number,
  data: PlanPayload
): Promise<ApiResponse<PlanRecord>> {
  const res = await api.put(`/api/subscription/admin/plans/${id}`, data)
  return res.data
}

export async function patchPlanStatus(
  id: number,
  enabled: boolean
): Promise<ApiResponse> {
  const res = await api.patch(`/api/subscription/admin/plans/${id}`, {
    enabled,
  })
  return res.data
}

// ============================================================================
// Admin User Subscription Management
// ============================================================================

export async function getUserSubscriptions(
  userId: number
): Promise<ApiResponse<UserSubscriptionRecord[]>> {
  const res = await api.get(
    `/api/subscription/admin/users/${userId}/subscriptions`
  )
  return res.data
}

export async function createUserSubscription(
  userId: number,
  data: CreateUserSubscriptionRequest
): Promise<ApiResponse<{ message?: string }>> {
  const res = await api.post(
    `/api/subscription/admin/users/${userId}/subscriptions`,
    data
  )
  return res.data
}

export async function invalidateUserSubscription(
  subId: number
): Promise<ApiResponse<{ message?: string }>> {
  const res = await api.post(
    `/api/subscription/admin/user_subscriptions/${subId}/invalidate`
  )
  return res.data
}

export async function deleteUserSubscription(
  subId: number
): Promise<ApiResponse> {
  const res = await api.delete(
    `/api/subscription/admin/user_subscriptions/${subId}`
  )
  return res.data
}

// ============================================================================
// User-facing Subscription Payment
// ============================================================================

export async function paySubscriptionStripe(
  data: SubscriptionPayRequest
): Promise<SubscriptionPayResponse> {
  const res = await api.post('/api/subscription/stripe/pay', data)
  return res.data
}

export async function paySubscriptionCreem(
  data: SubscriptionPayRequest
): Promise<SubscriptionPayResponse> {
  const res = await api.post('/api/subscription/creem/pay', data)
  return res.data
}

export async function paySubscriptionKyren(
  data: SubscriptionPayRequest
): Promise<SubscriptionKyrenPayResponse> {
  const res = await api.post('/api/subscription/kyren/pay', data)
  return res.data
}

export async function getSubscriptionKyrenProduct(
  planId: number
): Promise<SubscriptionKyrenProductResponse> {
  const res = await api.get(
    `/api/subscription/admin/plans/${planId}/kyren/product`
  )
  return res.data
}

export async function syncSubscriptionKyrenProduct(
  planId: number,
  mode: SubscriptionKyrenProductSyncMode
): Promise<SubscriptionKyrenProductSyncResponse> {
  const res = await api.post(
    `/api/subscription/admin/plans/${planId}/kyren/product`,
    { mode }
  )
  return res.data
}

export async function paySubscriptionEpay(
  data: SubscriptionPayRequest & { payment_method: string }
): Promise<SubscriptionPayResponse & { url?: string }> {
  const res = await api.post('/api/subscription/epay/pay', data)
  return {
    ...res.data,
    url: res.data.url || (res as unknown as { url?: string }).url,
  }
}

export function buildSubscriptionBalancePayRequestBody(
  data: SubscriptionBalancePayRequest
): SubscriptionBalancePayRequest {
  return {
    plan_id: data.plan_id,
    purchase_mode: data.purchase_mode,
    idempotency_key: data.idempotency_key,
  }
}

export async function paySubscriptionBalance(
  data: SubscriptionBalancePayRequest
): Promise<SubscriptionBalancePayResponse> {
  const res = await api.post(
    '/api/subscription/balance/pay',
    buildSubscriptionBalancePayRequestBody(data),
    { skipBusinessError: true } as Record<string, unknown>
  )
  return res.data
}

export async function getSubscriptionOrderStatus(
  tradeNo: string
): Promise<ApiResponse<SubscriptionOrderStatus>> {
  const res = await api.get(
    `/api/subscription/orders/${encodeURIComponent(tradeNo)}`,
    { skipBusinessError: true } as Record<string, unknown>
  )
  return res.data
}

// ============================================================================
// User Self Subscriptions
// ============================================================================

export async function getSelfSubscriptions(): Promise<
  ApiResponse<UserSubscriptionRecord[]>
> {
  const res = await api.get('/api/subscription/self')
  return res.data
}

export async function getSelfSubscriptionFull(): Promise<
  ApiResponse<SelfSubscriptionData>
> {
  const res = await api.get('/api/subscription/self')
  return res.data
}

export async function updateCodexProMode(
  data: UpdateCodexProModeRequest
): Promise<ApiResponse<UpdateCodexProModeResponse>> {
  const res = await api.put('/api/subscription/self/codex-pro-mode', data)
  return res.data
}

export async function updateSubscriptionBillingStrategy(
  data: UpdateSubscriptionBillingStrategyRequest
): Promise<ApiResponse<{ billing_strategy: SubscriptionBillingStrategy }>> {
  const res = await api.put('/api/subscription/self/billing-strategy', data)
  return res.data
}

export async function setActiveSubscription(
  data: SetActiveSubscriptionRequest
): Promise<ApiResponse<{ active_subscription_id: number }>> {
  const res = await api.put('/api/subscription/self/active', data)
  return res.data
}

export async function resetSubscriptionQuota(subscriptionId: number): Promise<
  ApiResponse<{
    subscription_id: number
    end_time: number
    next_reset_time?: number
  }>
> {
  const res = await api.post(
    `/api/subscription/self/${subscriptionId}/reset-quota`
  )
  return res.data
}

export async function getPublicPlans(): Promise<ApiResponse<PlanRecord[]>> {
  const res = await api.get('/api/subscription/plans')
  return res.data
}

export async function getHomePublicPlansQuiet(): Promise<
  ApiResponse<PublicPlanRecord[]>
> {
  try {
    const res = await api.get('/api/subscription/public/plans', {
      skipErrorHandler: true,
      skipBusinessError: true,
      disableDuplicate: true,
    } as Record<string, unknown>)
    return res.data
  } catch {
    return { success: false, data: [] }
  }
}
