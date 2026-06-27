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

// ============================================================================
// Billing mode
// ============================================================================

// '' means inherit (fall back to the selected channel's own billing profile).
export const CHANNEL_GROUP_BILLING_MODE = {
  INHERIT: '',
  USAGE_TOKENS: 'usage_tokens',
  FIXED_REQUEST: 'fixed_request',
} as const

export const DEFAULT_CHANNEL_GROUP_NAME = '__default__'

// ============================================================================
// API types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface ChannelGroup {
  id: number
  name: string
  description: string
  enabled: boolean
  credit_billing_mode: string
  fixed_request_credits: number
  dynamic_billing_multiplier_enabled: boolean
  token_billing_multiplier: number
  created_time: number
  updated_time: number
  channel_ids: number[]
  is_default: boolean
}

export interface ChannelGroupPayload {
  id?: number
  name: string
  description: string
  enabled: boolean
  credit_billing_mode: string
  fixed_request_credits: number
  dynamic_billing_multiplier_enabled: boolean
  token_billing_multiplier: number
  channel_ids: number[]
}

export interface ChannelOption {
  id: number
  name: string
}

// ============================================================================
// Form schema
// ============================================================================

export const channelGroupFormSchema = z
  .object({
    name: z.string().min(1, 'Name is required'),
    description: z.string(),
    enabled: z.boolean(),
    credit_billing_mode: z.enum(['', 'usage_tokens', 'fixed_request']),
    fixed_request_credits: z.number(),
    dynamic_billing_multiplier_enabled: z.boolean(),
    token_billing_multiplier: z.number(),
    channel_ids: z.array(z.number()),
  })
  .superRefine((value, ctx) => {
    if (
      value.credit_billing_mode === 'fixed_request' &&
      (!Number.isFinite(value.fixed_request_credits) ||
        value.fixed_request_credits <= 0)
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['fixed_request_credits'],
        message: 'Fixed request credits must be greater than 0',
      })
    }
  })

export type ChannelGroupFormValues = z.infer<typeof channelGroupFormSchema>

export const CHANNEL_GROUP_FORM_DEFAULTS: ChannelGroupFormValues = {
  name: '',
  description: '',
  enabled: true,
  credit_billing_mode: '',
  fixed_request_credits: 0,
  dynamic_billing_multiplier_enabled: false,
  token_billing_multiplier: 1,
  channel_ids: [],
}

export function channelGroupToFormValues(
  group: ChannelGroup
): ChannelGroupFormValues {
  return {
    name: group.name,
    description: group.description ?? '',
    enabled: group.enabled,
    credit_billing_mode: (group.credit_billing_mode === 'usage_tokens' ||
    group.credit_billing_mode === 'fixed_request'
      ? group.credit_billing_mode
      : '') as ChannelGroupFormValues['credit_billing_mode'],
    fixed_request_credits: group.fixed_request_credits ?? 0,
    dynamic_billing_multiplier_enabled:
      group.dynamic_billing_multiplier_enabled ?? false,
    token_billing_multiplier: group.token_billing_multiplier ?? 1,
    channel_ids: group.channel_ids ?? [],
  }
}

export function formValuesToPayload(
  data: ChannelGroupFormValues,
  id?: number
): ChannelGroupPayload {
  return {
    ...(id != null ? { id } : {}),
    name: data.name,
    description: data.description,
    enabled: data.enabled,
    credit_billing_mode: data.credit_billing_mode,
    fixed_request_credits:
      data.credit_billing_mode === 'fixed_request'
        ? data.fixed_request_credits
        : 0,
    dynamic_billing_multiplier_enabled: data.dynamic_billing_multiplier_enabled,
    token_billing_multiplier: data.token_billing_multiplier || 1,
    channel_ids: data.channel_ids,
  }
}
