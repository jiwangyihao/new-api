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
  GPTAbuseCountEligibleFilter,
  GPTAbuseSortBy,
  GPTAbuseSortOrder,
  GPTAbuseStatusFilter,
} from './types'

export const GPT_ABUSE_DEFAULT_LIMIT = 20
export const GPT_ABUSE_DETAIL_LIMIT = 20
export const GPT_ABUSE_REASON_MAX_LENGTH = 255
export const GPT_ABUSE_DEFAULT_REASON = 'manual_review'

export const GPT_ABUSE_STATUS_OPTIONS = [
  'all',
  'active_suspended',
  'warning_only',
] as const satisfies readonly GPTAbuseStatusFilter[]

export const GPT_ABUSE_SEVERITY_OPTIONS = ['', 'high', 'medium', 'low'] as const
export const GPT_ABUSE_KIND_OPTIONS = [
  '',
  'cyber_policy',
  'high_risk_cyber_reroute',
  'invalid_prompt_safety',
  'content_policy_violation',
  'generic_policy_violation',
  'generic_abuse_security_warning',
] as const
export const GPT_ABUSE_SOURCE_OPTIONS = [
  '',
  'http_error',
  'sse_metadata',
  'sse_response_failed',
] as const

export const GPT_ABUSE_SORT_BY_OPTIONS = [
  'latest_warning_at',
  'warning_count',
  'effective_warning_count',
  'user_id',
] as const satisfies readonly GPTAbuseSortBy[]

export const GPT_ABUSE_SORT_ORDER_OPTIONS = [
  'desc',
  'asc',
] as const satisfies readonly GPTAbuseSortOrder[]

export const GPT_ABUSE_COUNT_ELIGIBLE_OPTIONS = [
  'all',
  'true',
  'false',
] as const satisfies readonly GPTAbuseCountEligibleFilter[]
