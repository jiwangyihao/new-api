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
import { type StatusBadgeProps } from '@/components/status-badge'
import { formatTokens } from '@/lib/format'

// ============================================================================
// API Key Status Configuration
// label values are i18n keys; use t(config.label) in components (e.g. StatusBadge)
// ============================================================================

export const API_KEY_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
  EXPIRED: 3,
  EXHAUSTED: 4,
} as const

export const API_KEY_STATUSES: Record<
  number,
  Pick<StatusBadgeProps, 'variant' | 'showDot'> & {
    label: string
    value: number
  }
> = {
  [API_KEY_STATUS.ENABLED]: {
    label: 'Enabled',
    variant: 'success',
    value: API_KEY_STATUS.ENABLED,
    showDot: true,
  },
  [API_KEY_STATUS.DISABLED]: {
    label: 'Disabled',
    variant: 'neutral',
    value: API_KEY_STATUS.DISABLED,
    showDot: true,
  },
  [API_KEY_STATUS.EXPIRED]: {
    label: 'Expired',
    variant: 'warning',
    value: API_KEY_STATUS.EXPIRED,
    showDot: true,
  },
  [API_KEY_STATUS.EXHAUSTED]: {
    label: 'Exhausted',
    variant: 'danger',
    value: API_KEY_STATUS.EXHAUSTED,
    showDot: true,
  },
} as const

const API_KEY_TOKEN_LIMIT_REACHED_STATUS: Pick<StatusBadgeProps, 'variant' | 'showDot'> & {
  label: string
  value: number
} = {
  label: 'Credit Limit Reached',
  variant: 'danger',
  value: API_KEY_STATUS.ENABLED,
  showDot: true,
}

type ApiKeyStatusSource = {
  status: number
  token_limit_enabled?: boolean
  token_limit?: number
  token_used?: number
  token_unlimited?: boolean
  credit_limit_enabled?: boolean
  credit_limit?: number
  credit_used?: number
  credit_unlimited?: boolean
}

export function getApiKeyStatusConfig(apiKey: ApiKeyStatusSource) {
  const limitEnabled =
    apiKey.credit_limit_enabled ?? apiKey.token_limit_enabled ?? false
  const unlimited = apiKey.credit_unlimited ?? apiKey.token_unlimited ?? true
  const limit = apiKey.credit_limit ?? apiKey.token_limit ?? 0
  const used = apiKey.credit_used ?? apiKey.token_used ?? 0

  if (
    apiKey.status === API_KEY_STATUS.ENABLED &&
    limitEnabled &&
    !unlimited &&
    limit > 0 &&
    used >= limit
  ) {
    return API_KEY_TOKEN_LIMIT_REACHED_STATUS
  }
  return API_KEY_STATUSES[apiKey.status]
}

export function formatApiKeyCreditCount(credits: number, unit?: string): string {
  if (credits === 0) return unit ? `0 ${unit}` : '0'
  const formatted = formatTokens(credits)
  if (formatted === '-') return unit ? `0 ${unit}` : '0'
  if (credits > 0 && credits < 1000 && unit) return `${formatted} ${unit}`
  return formatted
}

export const formatApiKeyTokenCount = formatApiKeyCreditCount

export const API_KEY_STATUS_OPTIONS = Object.values(API_KEY_STATUSES).map(
  (config) => ({
    label: config.label,
    value: String(config.value),
  })
)

// ============================================================================
// Default Values
// ============================================================================

export const DEFAULT_GROUP = '' as const

// ============================================================================
// Error Messages (i18n keys: use t(ERROR_MESSAGES.xxx) when displaying)
// ============================================================================

export const ERROR_MESSAGES = {
  UNEXPECTED: 'An unexpected error occurred',
  LOAD_FAILED: 'Failed to load API keys',
  SEARCH_FAILED: 'Failed to search API keys',
  CREATE_FAILED: 'Failed to create API key',
  UPDATE_FAILED: 'Failed to update API key',
  DELETE_FAILED: 'Failed to delete API key',
  BATCH_DELETE_FAILED: 'Failed to delete API keys',
  STATUS_UPDATE_FAILED: 'Failed to update API key status',
} as const

// ============================================================================
// Success Messages (i18n keys: use t(SUCCESS_MESSAGES.xxx) when displaying)
// ============================================================================

export const SUCCESS_MESSAGES = {
  API_KEY_CREATED: 'API Key created successfully',
  API_KEY_UPDATED: 'API Key updated successfully',
  API_KEY_DELETED: 'API Key deleted successfully',
  API_KEY_ENABLED: 'API Key enabled successfully',
  API_KEY_DISABLED: 'API Key disabled successfully',
} as const
