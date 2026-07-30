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
import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { formatRedemptionWalletValue } from '@/features/redemption-codes/lib'
import { redeemTopupCode } from '../api'
import type { RedemptionRequest, RedemptionResult } from '../types'

// ============================================================================
// Redemption Hook
// ============================================================================
export const REDEMPTION_ERROR_MESSAGE_KEYS = {
  redemption_mode_required:
    'Subscription redemption codes require a redemption mode',
  redemption_mode_invalid: 'Redemption mode must be timed or credit_balance',
  credit_balance_redemption_unavailable:
    'Credit balance redemption is currently unavailable',
  redemption_plan_ineligible:
    'This plan is not eligible for Credit balance redemption',
  redemption_already_used: 'This redemption code has been used',
} as const

export function getRedemptionErrorMessageKey(
  code?: string
): string | undefined {
  if (!code) return undefined
  return REDEMPTION_ERROR_MESSAGE_KEYS[
    code as keyof typeof REDEMPTION_ERROR_MESSAGE_KEYS
  ]
}

function getRedemptionErrorMessage(code?: string, fallback?: string): string {
  const key = getRedemptionErrorMessageKey(code)
  return key ? i18next.t(key) : fallback || i18next.t('Redemption failed')
}

export class RedemptionRequestError extends Error {
  code?: string

  constructor(message: string, code?: string) {
    super(message)
    this.name = 'RedemptionRequestError'
    this.code = code
  }
}

export function isRedemptionModeRequiredError(error: unknown): boolean {
  return (
    error instanceof RedemptionRequestError &&
    error.code === 'redemption_mode_required'
  )
}

interface InitialRedemptionOptions {
  key: string
  redeem: (request: RedemptionRequest) => Promise<RedemptionResult>
  onRedeemed: () => Promise<void> | void
  onModeRequired: () => void
  onError: (message: string) => void
  fallbackError: string
}

export async function submitInitialRedemption({
  key,
  redeem,
  onRedeemed,
  onModeRequired,
  onError,
  fallbackError,
}: InitialRedemptionOptions): Promise<void> {
  try {
    await redeem({ key })
    await onRedeemed()
  } catch (error) {
    if (isRedemptionModeRequiredError(error)) {
      onModeRequired()
      return
    }

    onError(error instanceof Error ? error.message : fallbackError)
  }
}

export function useRedemption() {
  const [redeeming, setRedeeming] = useState(false)

  const redeemCode = useCallback(
    async (request: RedemptionRequest): Promise<RedemptionResult> => {
      if (!request.key || request.key.trim() === '') {
        throw new RedemptionRequestError(
          i18next.t('Please enter a redemption code')
        )
      }

      try {
        setRedeeming(true)
        const response = await redeemTopupCode(request)
        const result = response.result

        if (response.success && result) {
          if (
            result.type === 'subscription' &&
            result.redemption_mode === 'credit_balance' &&
            result.credit_balance
          ) {
            toast.success(
              i18next.t(
                'Redemption successful! Gross Credit: {{gross}}, debt offset: {{debt}}, available Credit balance: {{available}}',
                {
                  gross: result.credit_balance.gross_credit,
                  debt: result.credit_balance.debt_offset,
                  available: result.credit_balance.available_credit,
                }
              )
            )
          } else if (result.type === 'subscription') {
            toast.success(
              i18next.t(
                'Redemption successful! Subscription activated: {{plan}}',
                {
                  plan: result.plan?.title || i18next.t('Subscription plan'),
                }
              )
            )
          } else {
            toast.success(
              i18next.t('Redemption successful! Added: {{quota}}', {
                quota: formatRedemptionWalletValue(
                  result.quota ?? response.data ?? 0
                ),
              })
            )
          }
          return result
        }

        throw new RedemptionRequestError(
          getRedemptionErrorMessage(response.code, response.message),
          response.code
        )
      } finally {
        setRedeeming(false)
      }
    },
    []
  )

  return {
    redeeming,
    redeemCode,
  }
}
