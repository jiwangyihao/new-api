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
import { getCurrencyDisplay } from '@/lib/currency'
import { formatPlanPrice } from './format'

export interface AccountBalancePaymentInput {
  accountBalanceQuota: number
  priceAmount: number
  currency?: string
}

export type AccountBalancePaymentDisabledReason =
  | 'unsupported_currency'
  | 'insufficient_balance'
  | null

export interface AccountBalancePaymentState {
  supported: boolean
  sufficient: boolean
  disabled: boolean
  disabledReason: AccountBalancePaymentDisabledReason
}

function getAccountBalanceQuotaPerUnit(): number {
  const { config } = getCurrencyDisplay()
  return config.quotaPerUnit > 0 ? config.quotaPerUnit : 500000
}

export function accountBalanceQuotaToCnyAmount(quota: number): number {
  if (!Number.isFinite(quota) || quota <= 0) return 0
  return quota / getAccountBalanceQuotaPerUnit()
}

export function formatAccountBalanceForPlanPurchase(quota: number): string {
  return formatPlanPrice(accountBalanceQuotaToCnyAmount(quota), 'CNY')
}

export function getAccountBalancePaymentState(
  input: AccountBalancePaymentInput
): AccountBalancePaymentState {
  const supported = input.currency?.toUpperCase() === 'CNY'
  const balanceAmount = accountBalanceQuotaToCnyAmount(input.accountBalanceQuota)
  const sufficient = supported && balanceAmount >= input.priceAmount

  return {
    supported,
    sufficient,
    disabled: !supported || !sufficient,
    disabledReason: !supported
      ? 'unsupported_currency'
      : sufficient
        ? null
        : 'insufficient_balance',
  }
}
