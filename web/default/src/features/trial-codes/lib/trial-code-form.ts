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
import type { TrialCode, TrialCodePayload } from '../types'

export const TRIAL_CODE_FORM_DEFAULTS = {
  code: '',
  plan_id: 0,
  enabled: true,
  max_redemptions: 0,
  expires_at: undefined as Date | undefined,
}

export const getTrialCodeFormSchema = (t: (key: string) => string) =>
  z.object({
    code: z.string().trim().min(1, t('Trial code is required')),
    plan_id: z.coerce.number().int().positive(t('Trial plan ID is required')),
    enabled: z.boolean(),
    max_redemptions: z.coerce
      .number()
      .int()
      .min(0, t('Maximum redemptions cannot be negative')),
    expires_at: z.date().optional(),
  })

export type TrialCodeFormValues = z.infer<
  ReturnType<typeof getTrialCodeFormSchema>
>

export function trialCodeToFormValues(
  trialCode: TrialCode
): TrialCodeFormValues {
  return {
    code: trialCode.code,
    plan_id: trialCode.plan_id,
    enabled: trialCode.enabled,
    max_redemptions: trialCode.max_redemptions,
    expires_at:
      trialCode.expires_at > 0
        ? new Date(trialCode.expires_at * 1000)
        : undefined,
  }
}

export function formValuesToTrialCodePayload(
  values: TrialCodeFormValues
): TrialCodePayload {
  return {
    code: values.code.trim(),
    plan_id: values.plan_id,
    enabled: values.enabled,
    max_redemptions: values.max_redemptions,
    expires_at: values.expires_at
      ? Math.floor(values.expires_at.getTime() / 1000)
      : 0,
  }
}
