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
import { api } from '@/lib/api'

import { buildUsageAnalyticsApiParams } from './lib/filters'
import type {
  ApiResponse,
  UsageAnalyticsBreakdownResponse,
  UsageAnalyticsCanonicalFilters,
  UsageAnalyticsSummaryResponse,
  UsageAnalyticsTimeseriesResponse,
} from './types'

function buildUsageAnalyticsUrl(path: string, filters: UsageAnalyticsCanonicalFilters): string {
  const params = buildUsageAnalyticsApiParams(filters)
  return `/api/usage-analytics/${path}?${params.toString()}`
}

export async function getUsageAnalyticsSummary(
  filters: UsageAnalyticsCanonicalFilters
): Promise<ApiResponse<UsageAnalyticsSummaryResponse>> {
  const res = await api.get<ApiResponse<UsageAnalyticsSummaryResponse>>(
    buildUsageAnalyticsUrl('summary', filters)
  )
  return res.data
}

export async function getUsageAnalyticsTimeseries(
  filters: UsageAnalyticsCanonicalFilters
): Promise<ApiResponse<UsageAnalyticsTimeseriesResponse>> {
  const res = await api.get<ApiResponse<UsageAnalyticsTimeseriesResponse>>(
    buildUsageAnalyticsUrl('timeseries', filters)
  )
  return res.data
}

export async function getUsageAnalyticsBreakdown(
  filters: UsageAnalyticsCanonicalFilters
): Promise<ApiResponse<UsageAnalyticsBreakdownResponse>> {
  const res = await api.get<ApiResponse<UsageAnalyticsBreakdownResponse>>(
    buildUsageAnalyticsUrl('breakdown', filters)
  )
  return res.data
}
