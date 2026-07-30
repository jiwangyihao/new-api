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
import type {
  LiveSubscriptionConversionQuote,
  SubscriptionConversionQuote,
} from './types'

const BLOCK_SECONDS = 31n * 24n * 60n * 60n
const COOLDOWN_SECONDS = 24n * 60n * 60n
const GRACE_SECONDS = 336n * 60n * 60n

function parseQuoteInteger(value: string, field: string): bigint {
  if (!/^-?\d+$/.test(value)) {
    throw new Error(`Invalid conversion quote integer ${field}`)
  }
  return BigInt(value)
}

function nonNegative(value: bigint): bigint {
  return value > 0n ? value : 0n
}

function minimum(left: bigint, right: bigint): bigint {
  return left < right ? left : right
}

export function deriveLiveConversionQuote(
  quote: SubscriptionConversionQuote,
  elapsedSeconds: bigint
): LiveSubscriptionConversionQuote {
  const elapsed = nonNegative(elapsedSeconds)
  const serverNow = parseQuoteInteger(quote.database_now, 'database_now')
  const databaseNow = serverNow + elapsed
  const startTime = parseQuoteInteger(quote.start_time, 'start_time')
  const endTime = parseQuoteInteger(quote.end_time, 'end_time')
  const lastGrantedAt = parseQuoteInteger(
    quote.last_granted_at,
    'last_granted_at'
  )
  const creditBasis = parseQuoteInteger(quote.credit_basis, 'credit_basis')
  const currentRemainingCredit = parseQuoteInteger(
    quote.current_remaining_credit,
    'current_remaining_credit'
  )
  const currentDebt = parseQuoteInteger(quote.current_debt, 'current_debt')
  if (creditBasis < 0n || currentRemainingCredit < 0n || currentDebt < 0n) {
    throw new Error('Conversion quote contains a negative Credit value')
  }

  const remainingSeconds = nonNegative(endTime - databaseNow)
  const full31DayBlocks = remainingSeconds / BLOCK_SECONDS
  const grossCredit = full31DayBlocks * creditBasis + currentRemainingCredit
  const estimatedDebtOffset = minimum(grossCredit, currentDebt)
  const netAvailableCredit = grossCredit - estimatedDebtOffset
  const cooldownRemainingSeconds =
    lastGrantedAt > 0n
      ? nonNegative(lastGrantedAt + COOLDOWN_SECONDS - databaseNow)
      : 0n
  const expired = endTime <= databaseNow
  const graceEnd = endTime + GRACE_SECONDS
  const withinGrace = expired && databaseNow <= graceEnd
  const graceRemainingSeconds = withinGrace
    ? nonNegative(graceEnd - databaseNow)
    : 0n
  const structuralReasons = quote.reason_codes.filter(
    (code) => code !== 'cooldown_active'
  )
  const canConfirm =
    structuralReasons.length === 0 &&
    !quote.calculation_error_code &&
    grossCredit > 0n &&
    cooldownRemainingSeconds === 0n &&
    (!expired || withinGrace)
  const category =
    expired && withinGrace
      ? 'expired_grace'
      : canConfirm
        ? 'convertible'
        : 'excluded'

  return {
    sourceSubscriptionId: quote.source_subscription_id,
    databaseNow,
    startTime,
    endTime,
    remainingSeconds,
    full31DayBlocks,
    creditBasis,
    currentRemainingCredit,
    grossCredit,
    currentDebt,
    estimatedDebtOffset,
    netAvailableCredit,
    cooldownRemainingSeconds,
    graceRemainingSeconds,
    expired,
    withinGrace,
    category,
    canConfirm,
    formula: `${full31DayBlocks} × ${creditBasis} + ${currentRemainingCredit} = ${grossCredit}`,
  }
}
