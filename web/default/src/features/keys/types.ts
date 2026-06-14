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
import type { ApiKeyCodexProMode } from '@/features/subscriptions/types'

// ============================================================================
// API Key Schema & Types
// ============================================================================

export const apiKeyCodexProModeSchema = z.enum([
  'inherit',
  'all',
  'flexible',
  'off',
])

export type { ApiKeyCodexProMode }

export const apiKeySchema = z.object({
  id: z.number(),
  name: z.string(),
  key: z.string(),
  status: z.number(), // 1: enabled, 2: disabled, 3: expired, 4: exhausted
  remain_quota: z.number(),
  used_quota: z.number(),
  unlimited_quota: z.boolean(),
  token_limit_enabled: z.boolean().default(false),
  token_limit: z.number().default(0),
  token_used: z.number().default(0),
  token_remaining: z.number().default(0),
  token_unlimited: z.boolean().default(true),
  credit_limit_enabled: z.boolean().optional(),
  credit_limit: z.number().optional(),
  credit_used: z.number().optional(),
  credit_remaining: z.number().optional(),
  credit_unlimited: z.boolean().optional(),
  credit_reset_at: z.number().optional(),
  expired_time: z.number(), // -1 for never expires
  created_time: z.number(),
  accessed_time: z.number(),
  model_limits_enabled: z.boolean(),
  model_limits: z.string().nullish().default(''),
  allow_ips: z.string().nullish().default(''),
  codex_pro_mode: apiKeyCodexProModeSchema.default('inherit'),
})

export type ApiKey = z.infer<typeof apiKeySchema>

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetApiKeysParams {
  p?: number
  size?: number
}

export interface GetApiKeysResponse {
  success: boolean
  message?: string
  data?: {
    items: ApiKey[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchApiKeysParams {
  keyword?: string
  token?: string
  p?: number
  size?: number
}

export interface ApiKeyFormData {
  name: string
  expired_time: number
  token_limit_enabled: boolean
  token_limit?: number
  credit_limit_enabled?: boolean
  credit_limit?: number
  model_limits_enabled: boolean
  model_limits: string
  allow_ips: string
  codex_pro_mode: ApiKeyCodexProMode
}

// ============================================================================
// Dialog Types
// ============================================================================

export type ApiKeysDialogType =
  | 'create'
  | 'update'
  | 'delete'
  | 'batch-delete'
  | 'cc-switch'
