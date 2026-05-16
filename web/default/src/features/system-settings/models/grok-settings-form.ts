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
import * as z from 'zod'
import type { ModelSettings, UpdateOptionRequest } from '../types'

const numberInput = (schema: z.ZodNumber) =>
  z.preprocess((value) => Number(value), schema)

export const grokSettingsFormSchema = z.object({
  grok: z.object({
    violation_deduction_enabled: z.boolean(),
    violation_deduction_amount: numberInput(z.number().min(0)),
  }),
})

export type GrokSettingsFormValues = z.infer<typeof grokSettingsFormSchema>

export type GrokSettingsOptionValues = Pick<
  ModelSettings,
  'grok.violation_deduction_enabled' | 'grok.violation_deduction_amount'
>

export function buildGrokFormDefaults(
  defaultValues: GrokSettingsOptionValues
): GrokSettingsFormValues {
  return {
    grok: {
      violation_deduction_enabled:
        defaultValues['grok.violation_deduction_enabled'] ?? true,
      violation_deduction_amount:
        defaultValues['grok.violation_deduction_amount'] ?? 0.05,
    },
  }
}

export function collectGrokSettingUpdates(
  values: GrokSettingsFormValues,
  defaultValues: GrokSettingsOptionValues
): UpdateOptionRequest[] {
  const updates: UpdateOptionRequest[] = []
  const enabled = values.grok.violation_deduction_enabled
  const amount = values.grok.violation_deduction_amount

  if (enabled !== defaultValues['grok.violation_deduction_enabled']) {
    updates.push({
      key: 'grok.violation_deduction_enabled',
      value: enabled,
    })
  }

  if (amount !== defaultValues['grok.violation_deduction_amount']) {
    updates.push({
      key: 'grok.violation_deduction_amount',
      value: amount,
    })
  }

  return updates
}
