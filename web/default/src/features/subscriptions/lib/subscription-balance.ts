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

export function accountBalanceCentsToCnyAmount(balanceCents: number): number {
  if (!Number.isFinite(balanceCents)) return 0
  return balanceCents / 100
}

export function accountBalanceCnyToCents(amountCny: number): number {
  if (!Number.isFinite(amountCny) || amountCny <= 0) return 0
  return Math.round(amountCny * 100)
}

export function accountBalanceQuotaToCnyAmount(quota: number): number {
  return accountBalanceCentsToCnyAmount(quota)
}

export function formatAccountBalanceForPlanPurchase(quota: number): string {
  return formatPlanPrice(accountBalanceCentsToCnyAmount(quota), 'CNY')
}

export function getAccountBalancePaymentState(
  input: AccountBalancePaymentInput
): AccountBalancePaymentState {
  const supported = input.currency?.toUpperCase() === 'CNY'
  const requiredBalanceCents = Math.round(input.priceAmount * 100)
  const sufficient =
    supported &&
    Number.isFinite(requiredBalanceCents) &&
    input.accountBalanceQuota >= requiredBalanceCents

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
