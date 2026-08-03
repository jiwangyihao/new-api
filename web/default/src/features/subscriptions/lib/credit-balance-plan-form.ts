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
import { updateCreditBalancePlan } from '../api'
import type {
  ApiResponse,
  CreditBalancePlanUpdateRequest,
  SubscriptionPlan,
} from '../types'

export type CreditBalancePlanFormValues = CreditBalancePlanUpdateRequest

export function creditBalancePlanToFormValues(
  plan: SubscriptionPlan
): CreditBalancePlanFormValues {
  return {
    concurrency_limit: Number(plan.concurrency_limit || 0),
    queue_capacity: Number(plan.queue_capacity || 0),
    business_code: plan.business_code || '',
    configured: plan.credit_balance_configured === true,
    purchase_enabled: plan.credit_balance_purchase_enabled === true,
    redemption_enabled: plan.credit_balance_redemption_enabled === true,
    conversion_enabled: plan.credit_balance_conversion_enabled === true,
    valuation_currency:
      plan.valuation_currency?.toUpperCase() === 'USD' ? 'USD' : 'CNY',
  }
}

export function creditBalancePlanFormToRequest(
  values: CreditBalancePlanFormValues
): CreditBalancePlanUpdateRequest {
  return {
    concurrency_limit: Number(values.concurrency_limit || 0),
    queue_capacity: Number(values.queue_capacity || 0),
    business_code: values.business_code.trim(),
    configured: values.configured,
    purchase_enabled: values.configured && values.purchase_enabled,
    redemption_enabled: values.configured && values.redemption_enabled,
    conversion_enabled: values.configured && values.conversion_enabled,
    valuation_currency: values.valuation_currency,
  }
}

export async function submitCreditBalancePlanForm(
  values: CreditBalancePlanFormValues,
  submit: (
    payload: CreditBalancePlanUpdateRequest
  ) => Promise<ApiResponse<SubscriptionPlan>> = updateCreditBalancePlan
): Promise<ApiResponse<SubscriptionPlan>> {
  return submit(creditBalancePlanFormToRequest(values))
}
