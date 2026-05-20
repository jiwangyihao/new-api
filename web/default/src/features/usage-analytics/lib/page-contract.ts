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
import { buildUsageLogsDrilldownSearch } from './filters'
import { USAGE_ANALYTICS_DEFAULT_RANGE_SECONDS } from '../constants'
import type {
  UsageAnalyticsCanonicalFilters,
  UsageAnalyticsDrilldown,
  UsageLogsDrilldownSearch,
} from '../types'

export interface UsageAnalyticsQueryKeys {
  summary: readonly ['usage-analytics', 'summary', UsageAnalyticsCanonicalFilters]
  timeseries: readonly [
    'usage-analytics',
    'timeseries',
    UsageAnalyticsCanonicalFilters,
  ]
  breakdown: readonly [
    'usage-analytics',
    'breakdown',
    UsageAnalyticsCanonicalFilters,
  ]
}

export interface UsageAnalyticsRankingDrilldownTarget {
  to: '/usage-logs/$section'
  params: { section: 'common' }
  search: UsageLogsDrilldownSearch
}

export function buildDefaultUsageAnalyticsFilters(
  nowSeconds: number
): UsageAnalyticsCanonicalFilters {
  return {
    start_timestamp: nowSeconds - USAGE_ANALYTICS_DEFAULT_RANGE_SECONDS,
    end_timestamp: nowSeconds,
    granularity: 'day',
    group_by: 'token',
    metric: 'total_tokens',
    token_ids: [],
    model_names: [],
    groups: [],
    streams: [],
    statuses: [],
    limit: 10,
    sort_by: 'total_tokens',
    sort_order: 'desc',
  }
}

export function buildUsageAnalyticsQueryKeys(
  filters: UsageAnalyticsCanonicalFilters
): UsageAnalyticsQueryKeys {
  return {
    summary: ['usage-analytics', 'summary', filters] as const,
    timeseries: ['usage-analytics', 'timeseries', filters] as const,
    breakdown: ['usage-analytics', 'breakdown', filters] as const,
  }
}

export function buildUsageAnalyticsRankingDrilldown(
  filters: UsageAnalyticsCanonicalFilters,
  drilldown: UsageAnalyticsDrilldown | null
): UsageAnalyticsRankingDrilldownTarget | null {
  if (drilldown === null) return null
  return {
    to: '/usage-logs/$section',
    params: { section: 'common' },
    search: buildUsageLogsDrilldownSearch(filters, drilldown),
  }
}
