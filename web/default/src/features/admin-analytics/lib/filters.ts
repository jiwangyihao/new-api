import { z } from 'zod'
import {
  ADMIN_ANALYTICS_DEFAULT_LIMIT,
  ADMIN_ANALYTICS_DEFAULT_RANGE_SECONDS,
  ADMIN_ANALYTICS_MAX_LIMIT,
} from '../constants'
import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsGranularity,
  AdminAnalyticsSearch,
  AdminAnalyticsSortOrder,
  AdminAnalyticsSource,
  AdminAnalyticsTab,
  AdminPlanAttribution,
  AdminUsageGroupBy,
  AdminUsageMetric,
} from '../types'

const tabs = new Set<string>([
  'overview',
  'plans',
  'quota',
  'users',
  'conversion',
  'invitations',
  'usage',
  'risks',
])
const granularities = new Set<string>(['hour', 'day', 'week', 'month'])
const sortOrders = new Set<string>(['asc', 'desc'])
const sources = new Set<string>([
  'order',
  'trial_code',
  'invite_trial',
  'monthly_invite_entitlement',
  'admin',
  'redemption',
  'system',
  'unknown',
])
const usageGroupBy = new Set<string>([
  'user',
  'plan',
  'model',
  'stream',
  'status',
  'channel',
  'endpoint',
  'billing_source',
  'token',
  'subscription_source',
])
const usageMetrics = new Set<string>([
  'request_count',
  'total_tokens',
  'quota',
  'error_rate',
  'avg_latency_ms',
  'p95_latency_ms',
  'active_users',
  'active_api_keys',
])
const planAttributions = new Set<string>(['current', 'event_time'])

const looseSearchSchema = z.object({}).catchall(z.unknown())

export const adminAnalyticsSearchSchema: z.ZodType<AdminAnalyticsSearch> = z
  .unknown()
  .transform((search) => buildAdminAnalyticsCanonicalFilters(search))

function nowSeconds(): number {
  return Math.floor(Date.now() / 1000)
}

function firstString(value: unknown): string | undefined {
  if (Array.isArray(value)) {
    for (const item of value) {
      const scalar = scalarString(item)
      if (scalar !== undefined) return scalar
    }
    return undefined
  }
  return scalarString(value)
}

function scalarString(value: unknown): string | undefined {
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean')
    return String(value)
  return undefined
}

function parseInteger(value: unknown): number | undefined {
  const raw = firstString(value)?.trim()
  if (raw === undefined || !/^-?\d+$/.test(raw)) return undefined
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) ? parsed : undefined
}

function parseEnum<T extends string>(
  value: unknown,
  allowed: ReadonlySet<string>,
  fallback: T
): T {
  const raw = firstString(value)?.trim()
  return raw !== undefined && allowed.has(raw) ? (raw as T) : fallback
}

function normalizeArray(value: unknown): string[] {
  const raw = Array.isArray(value) ? value : value === undefined ? [] : [value]
  return Array.from(
    new Set(
      raw
        .flatMap((item) => scalarString(item)?.split(',') ?? [])
        .map((item) => item.trim())
        .filter(Boolean)
    )
  ).sort()
}

function parseIntArray(value: unknown): number[] {
  return Array.from(
    new Set(
      normalizeArray(value)
        .filter((item) => /^\d+$/.test(item))
        .map((item) => Number(item))
        .filter((item) => Number.isSafeInteger(item) && item > 0)
    )
  ).sort((first, second) => first - second)
}

function parseAllowedArray<T extends string>(
  value: unknown,
  allowed: ReadonlySet<string>
): T[] {
  return normalizeArray(value).filter((item) => allowed.has(item)) as T[]
}

function parseLimit(value: unknown): number {
  const parsed = parseInteger(value)
  if (parsed === undefined || parsed <= 0) return ADMIN_ANALYTICS_DEFAULT_LIMIT
  return Math.min(parsed, ADMIN_ANALYTICS_MAX_LIMIT)
}

export function buildAdminAnalyticsCanonicalFilters(
  search: unknown,
  currentSeconds = nowSeconds()
): AdminAnalyticsCanonicalFilters {
  const parsed = looseSearchSchema.safeParse(search)
  const raw = parsed.success ? parsed.data : {}
  const endTimestamp = parseInteger(raw.end_timestamp) ?? currentSeconds
  const startTimestamp =
    parseInteger(raw.start_timestamp) ??
    endTimestamp - ADMIN_ANALYTICS_DEFAULT_RANGE_SECONDS
  const metric = parseEnum<AdminUsageMetric>(
    raw.metric,
    usageMetrics,
    'total_tokens'
  )
  return {
    tab: parseEnum<AdminAnalyticsTab>(raw.tab, tabs, 'overview'),
    start_timestamp: startTimestamp,
    end_timestamp: endTimestamp,
    granularity: parseEnum<AdminAnalyticsGranularity>(
      raw.granularity,
      granularities,
      'day'
    ),
    plan_ids: parseIntArray(raw.plan_ids),
    sources: parseAllowedArray<AdminAnalyticsSource>(raw.sources, sources),
    statuses: normalizeArray(raw.statuses),
    group_by: parseEnum<AdminUsageGroupBy>(raw.group_by, usageGroupBy, 'user'),
    metric,
    plan_attribution: parseEnum<AdminPlanAttribution>(
      raw.plan_attribution,
      planAttributions,
      'current'
    ),
    top_n: parseLimit(raw.top_n),
    limit: parseLimit(raw.limit),
    offset: Math.max(parseInteger(raw.offset) ?? 0, 0),
    sort_by: firstString(raw.sort_by)?.trim() || undefined,
    sort_order: parseEnum<AdminAnalyticsSortOrder>(
      raw.sort_order,
      sortOrders,
      'desc'
    ),
  }
}

function appendArray(
  params: URLSearchParams,
  key: string,
  values: readonly (string | number)[]
): void {
  for (const value of values) params.append(key, String(value))
}

export function buildAdminAnalyticsApiParams(
  filters: AdminAnalyticsCanonicalFilters,
  options: { includeUsage?: boolean; includeSort?: boolean } = {}
): URLSearchParams {
  const params = new URLSearchParams()
  params.append('start_timestamp', String(filters.start_timestamp))
  params.append('end_timestamp', String(filters.end_timestamp))
  params.append('granularity', filters.granularity)
  params.append('limit', String(filters.limit))
  params.append('offset', String(filters.offset))
  params.append('sort_order', filters.sort_order)
  appendArray(params, 'plan_ids', filters.plan_ids)
  appendArray(params, 'sources', filters.sources)
  appendArray(params, 'statuses', filters.statuses)
  if (options.includeUsage === true) {
    params.append('group_by', filters.group_by)
    params.append('metric', filters.metric)
    params.append('plan_attribution', filters.plan_attribution)
    params.append('top_n', String(filters.top_n))
  }
  if (options.includeSort === true && filters.sort_by !== undefined) {
    params.append('sort_by', filters.sort_by)
  }
  return params
}
