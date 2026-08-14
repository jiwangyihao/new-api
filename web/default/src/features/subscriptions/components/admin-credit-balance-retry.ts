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
import type { CreditBalanceAdjustmentOperation } from '../types'

export interface CreditAdjustmentFacts {
  userId: number
  operation: CreditBalanceAdjustmentOperation
  amount: string
  planId: string
  reason: string
}

export interface CreditAdjustmentRetryState {
  facts: CreditAdjustmentFacts
  idempotencyKey: string
}

export type CreditAdjustmentRetryEvent = 'retry' | 'success'

function sameCreditAdjustmentFacts(
  left: CreditAdjustmentFacts,
  right: CreditAdjustmentFacts
): boolean {
  return (
    left.userId === right.userId &&
    left.operation === right.operation &&
    left.amount === right.amount &&
    left.planId === right.planId &&
    left.reason === right.reason
  )
}

export function reconcileCreditAdjustmentRetry(
  previous: CreditAdjustmentRetryState | null,
  facts: CreditAdjustmentFacts,
  event: CreditAdjustmentRetryEvent,
  createIdempotencyKey: () => string
): CreditAdjustmentRetryState {
  if (
    event === 'retry' &&
    previous !== null &&
    sameCreditAdjustmentFacts(previous.facts, facts)
  ) {
    return previous
  }

  return {
    facts,
    idempotencyKey: createIdempotencyKey(),
  }
}
