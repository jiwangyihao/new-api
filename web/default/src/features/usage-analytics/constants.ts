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
  UsageAnalyticsGranularity,
  UsageAnalyticsGroupBy,
  UsageAnalyticsMetric,
  UsageAnalyticsSortOrder,
} from './types'

export const USAGE_ANALYTICS_DEFAULT_GROUP_BY: UsageAnalyticsGroupBy = 'token'
export const USAGE_ANALYTICS_DEFAULT_METRIC: UsageAnalyticsMetric =
  'total_tokens'
export const USAGE_ANALYTICS_DEFAULT_GRANULARITY: UsageAnalyticsGranularity =
  'day'
export const USAGE_ANALYTICS_DEFAULT_LIMIT = 10
export const USAGE_ANALYTICS_MAX_LIMIT = 50
export const USAGE_ANALYTICS_DEFAULT_SORT_ORDER: UsageAnalyticsSortOrder = 'desc'
export const USAGE_ANALYTICS_DEFAULT_RANGE_SECONDS = 7 * 24 * 60 * 60

export interface UsageAnalyticsOption<TValue extends string> {
  value: TValue
  labelKey: string
}

export const USAGE_ANALYTICS_GROUP_BY_OPTIONS: Array<
  UsageAnalyticsOption<UsageAnalyticsGroupBy>
> = [
  { value: 'token', labelKey: 'usageAnalytics.groupBy.token' },
  { value: 'model', labelKey: 'usageAnalytics.groupBy.model' },
  { value: 'group', labelKey: 'usageAnalytics.groupBy.group' },
  { value: 'stream', labelKey: 'usageAnalytics.groupBy.stream' },
  { value: 'status', labelKey: 'usageAnalytics.groupBy.status' },
]

export const USAGE_ANALYTICS_METRIC_OPTIONS: Array<
  UsageAnalyticsOption<UsageAnalyticsMetric>
> = [
  { value: 'request_count', labelKey: 'usageAnalytics.metric.requestCount' },
  { value: 'total_tokens', labelKey: 'usageAnalytics.metric.totalTokens' },
  { value: 'quota', labelKey: 'usageAnalytics.metric.quota' },
  { value: 'error_rate', labelKey: 'usageAnalytics.metric.errorRate' },
  { value: 'avg_latency_ms', labelKey: 'usageAnalytics.metric.avgLatencyMs' },
  { value: 'p95_latency_ms', labelKey: 'usageAnalytics.metric.p95LatencyMs' },
]

export const USAGE_ANALYTICS_GRANULARITY_OPTIONS: Array<
  UsageAnalyticsOption<UsageAnalyticsGranularity>
> = [
  { value: 'hour', labelKey: 'usageAnalytics.granularity.hour' },
  { value: 'day', labelKey: 'usageAnalytics.granularity.day' },
]

export const USAGE_ANALYTICS_SORT_BY_OPTIONS: Array<
  UsageAnalyticsOption<UsageAnalyticsMetric | 'first_used_at' | 'last_used_at'>
> = [
  { value: 'request_count', labelKey: 'usageAnalytics.sortBy.requestCount' },
  { value: 'total_tokens', labelKey: 'usageAnalytics.sortBy.totalTokens' },
  { value: 'quota', labelKey: 'usageAnalytics.sortBy.quota' },
  { value: 'error_rate', labelKey: 'usageAnalytics.sortBy.errorRate' },
  { value: 'avg_latency_ms', labelKey: 'usageAnalytics.sortBy.avgLatencyMs' },
  { value: 'p95_latency_ms', labelKey: 'usageAnalytics.sortBy.p95LatencyMs' },
  { value: 'first_used_at', labelKey: 'usageAnalytics.sortBy.firstUsedAt' },
  { value: 'last_used_at', labelKey: 'usageAnalytics.sortBy.lastUsedAt' },
]

export const USAGE_ANALYTICS_SORT_ORDER_OPTIONS: Array<
  UsageAnalyticsOption<UsageAnalyticsSortOrder>
> = [
  { value: 'desc', labelKey: 'usageAnalytics.sortOrder.desc' },
  { value: 'asc', labelKey: 'usageAnalytics.sortOrder.asc' },
]
