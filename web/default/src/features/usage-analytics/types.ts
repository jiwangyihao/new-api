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
export type UsageAnalyticsGroupBy =
  | 'token'
  | 'model'
  | 'group'
  | 'stream'
  | 'status'

export type UsageAnalyticsMetric =
  | 'request_count'
  | 'total_tokens'
  | 'quota'
  | 'error_rate'
  | 'avg_latency_ms'
  | 'p95_latency_ms'

export type UsageAnalyticsGranularity = 'hour' | 'day'
export type UsageAnalyticsStatus = 'success' | 'error'
export type UsageAnalyticsSortOrder = 'asc' | 'desc'
export type UsageAnalyticsStream = 'true' | 'false'

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface UsageAnalyticsCanonicalFilters {
  start_timestamp: number
  end_timestamp: number
  granularity: UsageAnalyticsGranularity
  group_by: UsageAnalyticsGroupBy
  metric: UsageAnalyticsMetric
  token_ids: number[]
  model_names: string[]
  groups: string[]
  streams: UsageAnalyticsStream[]
  statuses: UsageAnalyticsStatus[]
  limit: number
  sort_by: string
  sort_order: UsageAnalyticsSortOrder
}

export type UsageAnalyticsSearch = UsageAnalyticsCanonicalFilters

export interface UsageAnalyticsDrilldown {
  token_id?: number
  model_name?: string
  group?: string
  is_stream?: boolean
  status?: UsageAnalyticsStatus
}

export interface UsageAnalyticsDimensionRef {
  group_by: UsageAnalyticsGroupBy
  group_key: string
  group_value: string
  group_label: string
  drilldown: UsageAnalyticsDrilldown | null
}

export interface UsageAnalyticsTokenInfo {
  id: number
  name: string
  masked_key: string | null
  status: number | null
  group: string | null
  remain_quota: number | null
  unlimited_quota: boolean | null
  deleted: boolean
}

export interface UsageAnalyticsBaseMetrics {
  request_count: number
  success_count: number
  error_count: number
  success_rate: number
  error_rate: number
  quota: number
  prompt_tokens: number
  completion_tokens: number
  metered_tokens: number
  total_tokens: number
  avg_latency_ms: number
  p95_latency_ms: number
}

export interface UsageAnalyticsMetrics extends UsageAnalyticsBaseMetrics {
  first_used_at: number
  last_used_at: number
  rpm: number
  tpm: number
  active_key_count: number
}

export interface UsageAnalyticsGroup
  extends UsageAnalyticsDimensionRef,
    UsageAnalyticsBaseMetrics {
  first_used_at: number
  last_used_at: number
  share: number | null
  token: UsageAnalyticsTokenInfo | null
}

export interface UsageAnalyticsSummaryResponse {
  total: UsageAnalyticsMetrics
  groups: UsageAnalyticsGroup[]
}

export interface UsageAnalyticsTimeseriesPoint
  extends UsageAnalyticsDimensionRef,
    UsageAnalyticsBaseMetrics {
  timestamp: number
  time_label: string
}

export interface UsageAnalyticsTimeseriesResponse {
  points: UsageAnalyticsTimeseriesPoint[]
  granularity: UsageAnalyticsGranularity
}

export interface UsageAnalyticsBreakdownResponse {
  groups: UsageAnalyticsGroup[]
  total_groups: number
  other: UsageAnalyticsGroup | null
  sort_by: string
  sort_order: UsageAnalyticsSortOrder
}

export interface UsageLogsDrilldownSearch {
  startTime: number
  endTime: number
  tokenId?: number
  model?: string
  group?: string
  isStream?: boolean
  status?: UsageAnalyticsStatus
}
