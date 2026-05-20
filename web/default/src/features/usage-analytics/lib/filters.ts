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
  USAGE_ANALYTICS_DEFAULT_GRANULARITY,
  USAGE_ANALYTICS_DEFAULT_GROUP_BY,
  USAGE_ANALYTICS_DEFAULT_LIMIT,
  USAGE_ANALYTICS_DEFAULT_METRIC,
  USAGE_ANALYTICS_DEFAULT_RANGE_SECONDS,
  USAGE_ANALYTICS_DEFAULT_SORT_ORDER,
  USAGE_ANALYTICS_MAX_LIMIT,
} from '../constants'
import type {
  UsageAnalyticsCanonicalFilters,
  UsageAnalyticsDrilldown,
  UsageAnalyticsGranularity,
  UsageAnalyticsGroupBy,
  UsageAnalyticsMetric,
  UsageAnalyticsSearch,
  UsageAnalyticsSortOrder,
  UsageAnalyticsStatus,
  UsageAnalyticsStream,
  UsageLogsDrilldownSearch,
} from '../types'

const usageAnalyticsGroupByValues = [
  'token',
  'model',
  'group',
  'stream',
  'status',
] as const
const usageAnalyticsMetricValues = [
  'request_count',
  'total_tokens',
  'quota',
  'error_rate',
  'avg_latency_ms',
  'p95_latency_ms',
] as const
const usageAnalyticsGranularityValues = ['hour', 'day'] as const
const usageAnalyticsStatusValues = ['success', 'error'] as const
const usageAnalyticsStreamValues = ['true', 'false'] as const
const usageAnalyticsSortOrderValues = ['asc', 'desc'] as const

const usageAnalyticsGroupBySet = new Set<string>(usageAnalyticsGroupByValues)
const usageAnalyticsMetricSet = new Set<string>(usageAnalyticsMetricValues)
const usageAnalyticsGranularitySet = new Set<string>(
  usageAnalyticsGranularityValues
)
const usageAnalyticsStatusSet = new Set<string>(usageAnalyticsStatusValues)
const usageAnalyticsStreamSet = new Set<string>(usageAnalyticsStreamValues)
const usageAnalyticsSortOrderSet = new Set<string>(usageAnalyticsSortOrderValues)
const usageAnalyticsSortBySet = new Set<string>([
  ...usageAnalyticsMetricValues,
  'first_used_at',
  'last_used_at',
])

const usageAnalyticsLooseSearchSchema = z
  .object({
    start_timestamp: z.unknown().optional(),
    end_timestamp: z.unknown().optional(),
    granularity: z.unknown().optional(),
    group_by: z.unknown().optional(),
    metric: z.unknown().optional(),
    token_ids: z.unknown().optional(),
    model_names: z.unknown().optional(),
    groups: z.unknown().optional(),
    streams: z.unknown().optional(),
    statuses: z.unknown().optional(),
    limit: z.unknown().optional(),
    sort_by: z.unknown().optional(),
    sort_order: z.unknown().optional(),
  })
  .catchall(z.unknown())

export const usageAnalyticsSearchSchema: z.ZodType<UsageAnalyticsSearch> = z
  .unknown()
  .transform((search) => normalizeUsageAnalyticsSearch(search))

function currentUnixSeconds(): number {
  return Math.floor(Date.now() / 1000)
}

function firstString(value: unknown): string | undefined {
  if (Array.isArray(value)) {
    for (const item of value) {
      if (typeof item === 'string') return item
      if (typeof item === 'number' || typeof item === 'boolean') {
        return String(item)
      }
    }
    return undefined
  }
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return undefined
}

function parseInteger(value: unknown): number | undefined {
  if (typeof value === 'number') {
    return Number.isSafeInteger(value) ? value : undefined
  }
  const rawValue = firstString(value)?.trim()
  if (rawValue === undefined || !/^-?\d+$/.test(rawValue)) return undefined
  const parsed = Number(rawValue)
  return Number.isSafeInteger(parsed) ? parsed : undefined
}

function parseEnum<TValue extends string>(
  value: unknown,
  allowed: ReadonlySet<string>,
  fallback: TValue
): TValue {
  const rawValue = firstString(value)?.trim()
  if (rawValue !== undefined && allowed.has(rawValue)) return rawValue as TValue
  return fallback
}

function parseLimit(value: unknown): number {
  const parsed = parseInteger(value)
  if (parsed === undefined || parsed < 1) return USAGE_ANALYTICS_DEFAULT_LIMIT
  return Math.min(parsed, USAGE_ANALYTICS_MAX_LIMIT)
}

function scalarValueToString(value: unknown): string | undefined {
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return undefined
}

function normalizeScalarValues(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.flatMap((item) => scalarValueToString(item) ?? [])
  }
  const scalar = scalarValueToString(value)
  if (scalar === undefined) return []
  return typeof value === 'string' ? scalar.split(',') : [scalar]
}

function uniqueSortedStrings(values: string[]): string[] {
  return Array.from(
    new Set(values.map((value) => value.trim()).filter(Boolean))
  ).sort()
}

function parseStringList(value: unknown): string[] {
  return uniqueSortedStrings(normalizeScalarValues(value))
}

function parseTokenIds(value: unknown): number[] {
  const ids = normalizeScalarValues(value)
    .filter((item) => /^\d+$/.test(item.trim()))
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isSafeInteger(item) && item > 0)
  return Array.from(new Set(ids)).sort((first, second) => first - second)
}

function parseAllowedList<TValue extends string>(
  value: unknown,
  allowed: ReadonlySet<string>
): TValue[] {
  return uniqueSortedStrings(normalizeScalarValues(value)).filter((item) =>
    allowed.has(item)
  ) as TValue[]
}

function normalizeRawSearch(search: unknown): Record<string, unknown> {
  const parsed = usageAnalyticsLooseSearchSchema.safeParse(search)
  if (!parsed.success) return {}
  return parsed.data
}

export function normalizeUsageAnalyticsSearch(
  search: unknown,
  nowSeconds: number = currentUnixSeconds()
): UsageAnalyticsCanonicalFilters {
  const rawSearch = normalizeRawSearch(search)
  const metric = parseEnum<UsageAnalyticsMetric>(
    rawSearch.metric,
    usageAnalyticsMetricSet,
    USAGE_ANALYTICS_DEFAULT_METRIC
  )
  const sortBy = parseEnum<string>(
    rawSearch.sort_by,
    usageAnalyticsSortBySet,
    metric
  )
  const endTimestamp = parseInteger(rawSearch.end_timestamp) ?? nowSeconds
  const startTimestamp =
    parseInteger(rawSearch.start_timestamp) ??
    endTimestamp - USAGE_ANALYTICS_DEFAULT_RANGE_SECONDS

  return {
    start_timestamp: startTimestamp,
    end_timestamp: endTimestamp,
    granularity: parseEnum<UsageAnalyticsGranularity>(
      rawSearch.granularity,
      usageAnalyticsGranularitySet,
      USAGE_ANALYTICS_DEFAULT_GRANULARITY
    ),
    group_by: parseEnum<UsageAnalyticsGroupBy>(
      rawSearch.group_by,
      usageAnalyticsGroupBySet,
      USAGE_ANALYTICS_DEFAULT_GROUP_BY
    ),
    metric,
    token_ids: parseTokenIds(rawSearch.token_ids),
    model_names: parseStringList(rawSearch.model_names),
    groups: parseStringList(rawSearch.groups),
    streams: parseAllowedList<UsageAnalyticsStream>(
      rawSearch.streams,
      usageAnalyticsStreamSet
    ),
    statuses: parseAllowedList<UsageAnalyticsStatus>(
      rawSearch.statuses,
      usageAnalyticsStatusSet
    ),
    limit: parseLimit(rawSearch.limit),
    sort_by: sortBy,
    sort_order: parseEnum<UsageAnalyticsSortOrder>(
      rawSearch.sort_order,
      usageAnalyticsSortOrderSet,
      USAGE_ANALYTICS_DEFAULT_SORT_ORDER
    ),
  }
}

export function buildUsageAnalyticsCanonicalFilters(
  search: unknown,
  nowSeconds?: number
): UsageAnalyticsCanonicalFilters {
  return normalizeUsageAnalyticsSearch(search, nowSeconds)
}

function appendNumberParam(
  params: URLSearchParams,
  key: string,
  value: number
): void {
  params.append(key, String(value))
}

function appendStringArrayParams(
  params: URLSearchParams,
  key: string,
  values: string[]
): void {
  for (const value of values) params.append(key, value)
}

function appendNumberArrayParams(
  params: URLSearchParams,
  key: string,
  values: number[]
): void {
  for (const value of values) params.append(key, String(value))
}

export function buildUsageAnalyticsApiParams(
  filters: UsageAnalyticsCanonicalFilters
): URLSearchParams {
  const params = new URLSearchParams()
  appendNumberParam(params, 'start_timestamp', filters.start_timestamp)
  appendNumberParam(params, 'end_timestamp', filters.end_timestamp)
  params.append('granularity', filters.granularity)
  params.append('group_by', filters.group_by)
  params.append('metric', filters.metric)
  appendNumberArrayParams(params, 'token_ids', filters.token_ids)
  appendStringArrayParams(params, 'model_names', filters.model_names)
  appendStringArrayParams(params, 'groups', filters.groups)
  appendStringArrayParams(params, 'streams', filters.streams)
  appendStringArrayParams(params, 'statuses', filters.statuses)
  appendNumberParam(params, 'limit', filters.limit)
  params.append('sort_by', filters.sort_by)
  params.append('sort_order', filters.sort_order)
  return params
}

export function buildUsageLogsDrilldownSearch(
  filters: Pick<
    UsageAnalyticsCanonicalFilters,
    'start_timestamp' | 'end_timestamp'
  >,
  drilldown: UsageAnalyticsDrilldown
): UsageLogsDrilldownSearch {
  const tokenId = drilldown.token_id
  return {
    startTime: filters.start_timestamp * 1000,
    endTime: filters.end_timestamp * 1000,
    ...(tokenId !== undefined && Number.isSafeInteger(tokenId) && tokenId > 0
      ? { tokenId }
      : {}),
    ...(drilldown.model_name !== undefined
      ? { model: drilldown.model_name }
      : {}),
    ...(drilldown.group !== undefined ? { group: drilldown.group } : {}),
    ...(drilldown.is_stream !== undefined
      ? { isStream: drilldown.is_stream }
      : {}),
    ...(drilldown.status ? { status: drilldown.status } : {}),
  }
}

export function buildApiKeyUsageAnalyticsSearch(input: {
  id: number
  name?: string
  key?: string
}): Pick<UsageAnalyticsCanonicalFilters, 'group_by' | 'token_ids'> {
  return { group_by: 'token', token_ids: [input.id] }
}
