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

export type GPTAbuseStatusFilter = 'all' | 'active_suspended' | 'warning_only'
export type GPTAbuseSortBy =
  | 'warning_count'
  | 'effective_warning_count'
  | 'latest_warning_at'
  | 'user_id'
export type GPTAbuseSortOrder = 'asc' | 'desc'
export type GPTAbuseCountEligibleFilter = 'all' | 'true' | 'false'

export type GPTAbuseUserListSearch = {
  start_timestamp?: number
  end_timestamp?: number
  keyword: string
  user_id?: number
  status: GPTAbuseStatusFilter
  kind: string
  severity: string
  source: string
  limit: number
  offset: number
  sort_by: GPTAbuseSortBy
  sort_order: GPTAbuseSortOrder
}

export type GPTAbuseLogSearch = {
  start_timestamp?: number
  end_timestamp?: number
  source: string
  kind: string
  severity: string
  count_eligible: GPTAbuseCountEligibleFilter
  limit: number
  offset: number
}

export type GPTAbuseRepeatBlockSearch = {
  start_timestamp?: number
  end_timestamp?: number
  limit: number
  offset: number
}

export type GPTAbuseApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}

export type GPTAbuseActiveSuspension = {
  id: number
  reason: string
  suspended_until: number
  daily_count: number
  daily_limit: number
}

export type GPTAbuseUserListItem = {
  user_id: number
  username: string
  user_email: string
  warning_count: number
  effective_warning_count: number
  daily_limit: number
  remaining_warning_count: number
  high_count: number
  medium_count: number
  max_severity: string
  latest_warning_at: number
  latest_kind: string
  latest_source: string
  latest_requested_model: string
  latest_upstream_model: string
  latest_channel_id: number
  latest_channel_name: string
  suspension_status: string
  active_suspension: GPTAbuseActiveSuspension | null
  last_reset_at: number
  last_reset_by: number
  repeat_block_count: number
  latest_repeat_block_at: number
}

export type GPTAbuseUserListResponse = {
  items: GPTAbuseUserListItem[]
  total: number
  limit: number
  offset: number
  start_timestamp: number
  end_timestamp: number
}

export type GPTAbuseSignalLogItem = {
  id: number
  created_at: number
  user_id: number
  username: string
  user_email: string
  token_id: number
  token_name: string
  channel_id: number
  channel_name: string
  channel_type: number
  channel_multi_key_index: number
  request_id: string
  upstream_request_id: string
  endpoint: string
  relay_mode: number
  requested_model: string
  upstream_model: string
  is_stream: boolean
  source: string
  kind: string
  severity: string
  status_code: number
  error_code: string
  error_type: string
  count_eligible: boolean
  extra: unknown
}

export type GPTAbuseLogListResponse = {
  items: GPTAbuseSignalLogItem[]
  total: number
  limit: number
  offset: number
  start_timestamp: number
  end_timestamp: number
}

export type GPTAbuseRepeatBlockItem = {
  id: number
  created_at: number
  user_id: number
  username: string
  token_id: number
  token_name: string
  request_id: string
  endpoint: string
  relay_mode: number
  requested_model: string
  body_fingerprint_prefix: string
  first_warning_log_id: number
  first_warning_at: number
  first_warning_request_id: string
  first_warning_upstream_request_id: string
  first_warning_source: string
  first_warning_kind: string
  first_warning_severity: string
  channel_id: number
  channel_name: string
  channel_type: number
}

export type GPTAbuseRepeatBlockListResponse = {
  items: GPTAbuseRepeatBlockItem[]
  total: number
  limit: number
  offset: number
  start_timestamp: number
  end_timestamp: number
}

export type GPTAbuseReasonPayload = {
  reason: string
}

export type GPTAbuseResetWarningsPayload = {
  reason: string
  clear_suspension: boolean
}

export type GPTAbuseClearSuspensionResponse = {
  user_id: number
  had_active_suspension: boolean
  suspension_cleared: boolean
  cleared_suspension_id: number
}

export type GPTAbuseResetWarningsResponse = {
  reset_id: number
  user_id: number
  window_start: number
  window_end: number
  reset_at: number
  previous_raw_count: number
  previous_effective_count: number
  effective_warning_count: number
  cutoff_signal_log_id: number
  had_active_suspension: boolean
  suspension_cleared: boolean
  cleared_suspension_id: number
}
