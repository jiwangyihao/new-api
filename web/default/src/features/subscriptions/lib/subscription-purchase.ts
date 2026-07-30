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
import type { TFunction } from 'i18next'
import type {
  CreditBalanceGrantResult,
  PlanRecord,
  SubscriptionPurchaseMode,
} from '../types'

export const purchaseModeSchema = z.object({
  purchase_mode: z.enum(['timed', 'credit_balance']),
})

export type PurchaseModeFormValues = z.infer<typeof purchaseModeSchema>

export function isCreditBalancePurchaseAvailable(
  plan: PlanRecord['plan'] | null | undefined,
  globallyEnabled: boolean
): boolean {
  return !!(
    globallyEnabled &&
    plan?.enabled &&
    plan.public_visible &&
    plan.unlimited_purchase_enabled &&
    plan.duration_unit === 'month' &&
    Number(plan.duration_value) === 1 &&
    plan.quota_reset_period === 'monthly' &&
    Number(plan.monthly_token_limit || 0) > 0 &&
    !plan.is_trial &&
    !plan.invite_trial
  )
}

export function initialSubscriptionPurchaseMode(
  preference: SubscriptionPurchaseMode | undefined,
  creditAvailable: boolean
): SubscriptionPurchaseMode | undefined {
  if (preference === 'credit_balance') {
    return creditAvailable ? preference : undefined
  }
  return preference === 'timed' ? preference : undefined
}

export function creditPurchaseSuccessMessage(
  grant: CreditBalanceGrantResult,
  t: TFunction
): string {
  return t(
    'Added {{gross}} Credits; offset {{debt}} debt; {{available}} Credits available.',
    {
      gross: grant.gross_credit,
      debt: grant.debt_offset,
      available: grant.available_credit,
    }
  )
}
