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
import type { ApiResponse } from '@/features/subscriptions/types'

export type ConversionQuoteCategory =
  | 'convertible'
  | 'expired_grace'
  | 'excluded'

export type ConversionQuoteCreditBasisSource =
  | 'grant_snapshot'
  | 'current_plan_fallback'
  | 'unavailable'

export interface SubscriptionConversionQuoteReason {
  code: string
  data?: Record<string, boolean | string>
}

export interface SubscriptionConversionQuote {
  source_subscription_id: string
  plan_id: string
  plan_title: string
  entitlement_type: string
  grant_source: string
  status: string
  category: ConversionQuoteCategory
  database_now: string
  start_time: string
  end_time: string
  remaining_seconds: string
  full_31_day_blocks: string
  credit_basis: string
  credit_basis_source: ConversionQuoteCreditBasisSource
  current_remaining_credit: string
  gross_credit: string
  current_debt: string
  estimated_debt_offset: string
  net_available_credit: string
  last_granted_at: string
  last_grant_time_source: string
  last_grant_source: string
  cooldown_status: 'ready' | 'active' | 'unknown'
  cooldown_remaining_seconds: string
  grace_status: 'not_started' | 'active' | 'expired'
  grace_remaining_seconds: string
  expired: boolean
  within_grace: boolean
  eligible: boolean
  can_confirm: boolean
  reason_codes: string[]
  reasons: SubscriptionConversionQuoteReason[]
  calculation_error_code?: string
}

export interface SubscriptionConversionHistory {
  id: string
  source_subscription_id: string
  source_plan_id: string
  source_plan_title: string
  target_subscription_id: string
  target_plan_id: string
  ledger_id: string
  source_status: string
  grant_source: string
  database_now: string
  source_start_time: string
  source_end_time: string
  remaining_seconds: string
  full_31_day_blocks: string
  credit_basis: string
  credit_basis_source: ConversionQuoteCreditBasisSource
  current_remaining_credit: string
  gross_credit: string
  debt_offset: string
  net_available_credit: string
  available_credit_after: string
  settlement_debt_after: string
  balance_before: string
  balance_after: string
  last_granted_at: string
  last_grant_time_source: string
  last_grant_source: string
  converted_at: string
}

export interface SubscriptionConversionConfirmRequest {
  subscription_id: string
  idempotency_key: string
}

export interface SubscriptionConversionConfirmResult {
  replayed: boolean
  conversion: SubscriptionConversionHistory
}

export interface SubscriptionConversionQuoteList {
  database_now: string
  quotes: SubscriptionConversionQuote[]
  conversions: SubscriptionConversionHistory[]
}

export type SubscriptionConversionQuoteResponse =
  ApiResponse<SubscriptionConversionQuoteList>

export type SubscriptionConversionConfirmResponse =
  ApiResponse<SubscriptionConversionConfirmResult>

export interface LiveSubscriptionConversionQuote {
  sourceSubscriptionId: string
  databaseNow: bigint
  startTime: bigint
  endTime: bigint
  remainingSeconds: bigint
  full31DayBlocks: bigint
  creditBasis: bigint
  currentRemainingCredit: bigint
  grossCredit: bigint
  currentDebt: bigint
  estimatedDebtOffset: bigint
  netAvailableCredit: bigint
  cooldownRemainingSeconds: bigint
  graceRemainingSeconds: bigint
  expired: boolean
  withinGrace: boolean
  category: ConversionQuoteCategory
  canConfirm: boolean
  formula: string
}
