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
import {
  apiKeyCodexProModeSchema,
  type ApiKeyFormData,
  type ApiKey,
} from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export const API_KEY_CODEX_PRO_MODE_OPTIONS = [
  {
    value: 'inherit',
    labelKey: 'Inherit user setting',
    descriptionKey: 'Use the user-level Codex Pro setting for this API key.',
  },
  {
    value: 'all',
    labelKey: 'All',
    descriptionKey:
      'All eligible GPT-family Responses requests try Codex Pro without requiring the intent header.',
  },
  {
    value: 'flexible',
    labelKey: 'Flexible',
    descriptionKey:
      'Only requests with X-NewAPI-Codex-Pro-Intent: codex-pro try Codex Pro in flexible mode.',
  },
  {
    value: 'off',
    labelKey: 'Off',
    descriptionKey:
      'Codex Pro is disabled; eligible requests stay on the normal group.',
  },
] as const

export const apiKeyFormSchema = z
  .object({
    name: z.string().min(1, 'Name is required'),
    expired_time: z.date().optional(),
    token_limit_enabled: z.boolean(),
    token_limit: z.custom<number | undefined>(
      (value) => value === undefined || typeof value === 'number',
      'Token limit must be greater than 0'
    ),
    model_limits: z.array(z.string()),
    allow_ips: z.string().optional(),
    codex_pro_mode: apiKeyCodexProModeSchema,
    tokenCount: z.number().min(1).optional(),
  })
  .superRefine((value, ctx) => {
    if (!value.token_limit_enabled) return
    if (
      value.token_limit == null ||
      !Number.isFinite(value.token_limit) ||
      !Number.isInteger(value.token_limit) ||
      value.token_limit <= 0
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['token_limit'],
        message: 'Token limit must be greater than 0',
      })
    }
  })

export type ApiKeyFormValues = z.infer<typeof apiKeyFormSchema>

// ============================================================================
// Form Defaults
// ============================================================================

export const API_KEY_FORM_DEFAULT_VALUES: ApiKeyFormValues = {
  name: '',
  expired_time: undefined,
  token_limit_enabled: false,
  token_limit: undefined,
  model_limits: [],
  allow_ips: '',
  codex_pro_mode: 'inherit',
  tokenCount: 1,
}

export function getApiKeyFormDefaultValues(): ApiKeyFormValues {
  return API_KEY_FORM_DEFAULT_VALUES
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: ApiKeyFormValues
): ApiKeyFormData {
  return {
    name: data.name,
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : -1,
    token_limit_enabled: data.token_limit_enabled,
    token_limit: data.token_limit_enabled ? data.token_limit : 0,
    model_limits_enabled: data.model_limits.length > 0,
    model_limits: data.model_limits.join(','),
    allow_ips: data.allow_ips || '',
    codex_pro_mode: data.codex_pro_mode,
  }
}

/**
 * Transform API key data to form defaults
 */
export function transformApiKeyToFormDefaults(
  apiKey: ApiKey
): ApiKeyFormValues {
  return {
    name: apiKey.name,
    expired_time:
      apiKey.expired_time > 0
        ? new Date(apiKey.expired_time * 1000)
        : undefined,
    token_limit_enabled: apiKey.token_limit_enabled ?? false,
    token_limit: apiKey.token_limit_enabled ? apiKey.token_limit : undefined,
    model_limits: apiKey.model_limits
      ? apiKey.model_limits.split(',').filter(Boolean)
      : [],
    allow_ips: apiKey.allow_ips || '',
    codex_pro_mode: apiKey.codex_pro_mode ?? 'inherit',
    tokenCount: 1,
  }
}
