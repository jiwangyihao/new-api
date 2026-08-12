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
  ADMIN_ANALYTICS_DEFAULT_LIMIT,
  ADMIN_ANALYTICS_DEFAULT_RANGE_SECONDS,
  ADMIN_ANALYTICS_MAX_LIMIT,
} from '../constants'
import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsExcludedMode,
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
  'paid-subscription-value',
  'users',
  'conversion',
  'invitations',
  'invitation-paid-subscriptions',
  'usage',
  'risks',
])
const granularities = new Set<string>(['hour', 'day', 'week', 'month'])
const sortOrders = new Set<string>(['asc', 'desc'])
const subscriptionStatuses = new Set<string>([
  'active',
  'expired',
  'cancelled',
  'inactive',
  'converted',
])
const userStatuses = new Set<string>(['enabled', 'disabled'])
const logStatuses = new Set<string>(['success', 'error'])
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
const excludedModes = new Set<string>([
  'included_only',
  'include_excluded',
  'excluded_only',
])

const adminAnalyticsMoneySorts = new Set<string>([
  'recognized_remaining_value',
  'plan_price',
  'recognized_invitation_paid_amount',
  'active_invitation_paid_amount',
  'active_invitation_remaining_value',
  'recognized_paid_amount',
  'active_remaining_value',
])
const adminAnalyticsNoLimit = 0

const paidSubscriptionAnalyticsTabs = new Set<AdminAnalyticsTab>([
  'paid-subscription-value',
  'invitation-paid-subscriptions',
])

export function switchAdminAnalyticsTab(
  filters: AdminAnalyticsCanonicalFilters,
  tab: AdminAnalyticsTab
): AdminAnalyticsCanonicalFilters {
  if (filters.tab === tab) return filters
  const {
    snapshot_at: _snapshotAt,
    subscription_id: _subscriptionID,
    inviter_id: _inviterID,
    invitee_id: _inviteeID,
    currency: _currency,
    ...rest
  } = filters
  if (paidSubscriptionAnalyticsTabs.has(tab)) {
    return {
      ...rest,
      tab,
      time_range_explicit: false,
      offset: 0,
      sort_by: undefined,
      limit: ADMIN_ANALYTICS_DEFAULT_LIMIT,
      top_n: ADMIN_ANALYTICS_DEFAULT_LIMIT,
      ...(filters.currency !== undefined ? { currency: filters.currency } : {}),
    }
  }
  return {
    ...rest,
    tab,
    excluded_mode: 'included_only',
    active_only: false,
    time_range_explicit: true,
    limit: ADMIN_ANALYTICS_DEFAULT_LIMIT,
    top_n: ADMIN_ANALYTICS_DEFAULT_LIMIT,
    offset: 0,
    sort_by: undefined,
  }
}

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
  const raw = firstString(value)?.trim().toLowerCase()
  if (raw === 'all' || raw === '0') return 0
  const parsed = parseInteger(value)
  if (parsed === undefined || parsed < 0) return ADMIN_ANALYTICS_DEFAULT_LIMIT
  if (parsed === 0) return 0
  return Math.min(parsed, ADMIN_ANALYTICS_MAX_LIMIT)
}

function clampAdminAnalyticsLimit(value: number): number {
  if (value <= 0) return adminAnalyticsNoLimit
  return Math.min(value, ADMIN_ANALYTICS_MAX_LIMIT)
}

export function enableAdminAnalyticsAllRows(
  filters: AdminAnalyticsCanonicalFilters
): AdminAnalyticsCanonicalFilters {
  return {
    ...filters,
    limit: adminAnalyticsNoLimit,
    top_n: adminAnalyticsNoLimit,
    offset: 0,
  }
}

export function enableAdminAnalyticsPagedRows(
  filters: AdminAnalyticsCanonicalFilters,
  pageSize = ADMIN_ANALYTICS_DEFAULT_LIMIT
): AdminAnalyticsCanonicalFilters {
  const limit = clampAdminAnalyticsLimit(pageSize)
  return {
    ...filters,
    limit: limit === adminAnalyticsNoLimit ? ADMIN_ANALYTICS_MAX_LIMIT : limit,
    top_n: limit === adminAnalyticsNoLimit ? ADMIN_ANALYTICS_MAX_LIMIT : limit,
    offset: 0,
  }
}
function parseBoolean(value: unknown, fallback: boolean): boolean {
  const raw = firstString(value)?.trim().toLowerCase()
  if (raw === undefined) return fallback
  if (raw === 'true' || raw === '1') return true
  if (raw === 'false' || raw === '0') return false
  return fallback
}

function parsePositiveInteger(value: unknown): number | undefined {
  const parsed = parseInteger(value)
  return parsed !== undefined && parsed > 0 ? parsed : undefined
}

function parseOptionalString(value: unknown): string | undefined {
  const raw = firstString(value)?.trim()
  return raw === undefined || raw === '' ? undefined : raw
}

function shouldIncludeTimeRange(
  filters: AdminAnalyticsCanonicalFilters,
  includeTimeRange: boolean
): boolean {
  if (includeTimeRange !== true) return false
  if (!paidSubscriptionAnalyticsTabs.has(filters.tab)) return true
  return filters.time_range_explicit
}

export function buildAdminAnalyticsCanonicalFilters(
  search: unknown,
  currentSeconds = nowSeconds()
): AdminAnalyticsCanonicalFilters {
  const parsed = looseSearchSchema.safeParse(search)
  const raw = parsed.success ? parsed.data : {}
  const tab = parseEnum<AdminAnalyticsTab>(raw.tab, tabs, 'overview')
  const endTimestamp = parseInteger(raw.end_timestamp) ?? currentSeconds
  const startTimestamp =
    parseInteger(raw.start_timestamp) ??
    endTimestamp - ADMIN_ANALYTICS_DEFAULT_RANGE_SECONDS
  const metric = parseEnum<AdminUsageMetric>(
    raw.metric,
    usageMetrics,
    'total_tokens'
  )
  const snapshotAt = parseInteger(raw.snapshot_at)
  const currency = parseOptionalString(raw.currency)
  const rawSortBy = firstString(raw.sort_by)?.trim() || undefined
  const sortBy =
    rawSortBy !== undefined &&
    !(currency === undefined && adminAnalyticsMoneySorts.has(rawSortBy))
      ? rawSortBy
      : undefined
  const limit = parseLimit(raw.limit)
  const topN = parseLimit(raw.top_n)
  return {
    tab,
    start_timestamp: startTimestamp,
    end_timestamp: endTimestamp,
    ...(snapshotAt !== undefined ? { snapshot_at: snapshotAt } : {}),
    granularity: parseEnum<AdminAnalyticsGranularity>(
      raw.granularity,
      granularities,
      'day'
    ),
    plan_ids: parseIntArray(raw.plan_ids),
    user_ids: parseIntArray(raw.user_ids),
    token_ids: parseIntArray(raw.token_ids),
    channel_ids: parseIntArray(raw.channel_ids),
    sources: parseAllowedArray<AdminAnalyticsSource>(raw.sources, sources),
    statuses: normalizeArray(raw.statuses),
    subscription_statuses: parseAllowedArray<string>(
      raw.subscription_statuses,
      subscriptionStatuses
    ),
    user_statuses: parseAllowedArray<string>(raw.user_statuses, userStatuses),
    log_statuses: parseAllowedArray<string>(raw.log_statuses, logStatuses),
    grant_reasons: parseAllowedArray<string>(raw.grant_reasons, sources),
    business_codes: normalizeArray(raw.business_codes),
    ...(currency !== undefined ? { currency } : {}),
    excluded_mode: parseEnum<AdminAnalyticsExcludedMode>(
      raw.excluded_mode,
      excludedModes,
      'included_only'
    ),
    active_only: parseBoolean(raw.active_only, false),
    time_range_explicit: parseBoolean(
      raw.time_range_explicit,
      !paidSubscriptionAnalyticsTabs.has(tab)
    ),
    ...(parsePositiveInteger(raw.inviter_id) !== undefined
      ? { inviter_id: parsePositiveInteger(raw.inviter_id) }
      : {}),
    ...(parsePositiveInteger(raw.invitee_id) !== undefined
      ? { invitee_id: parsePositiveInteger(raw.invitee_id) }
      : {}),
    ...(parsePositiveInteger(raw.subscription_id) !== undefined
      ? { subscription_id: parsePositiveInteger(raw.subscription_id) }
      : {}),
    group_by: parseEnum<AdminUsageGroupBy>(raw.group_by, usageGroupBy, 'user'),
    metric,
    plan_attribution: parseEnum<AdminPlanAttribution>(
      raw.plan_attribution,
      planAttributions,
      'current'
    ),
    top_n: clampAdminAnalyticsLimit(topN),
    limit: clampAdminAnalyticsLimit(limit),
    offset: Math.max(parseInteger(raw.offset) ?? 0, 0),
    sort_by: sortBy,
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
  options: {
    includeTimeRange?: boolean
    includeSubscriptionID?: boolean
    includeUsage?: boolean
    includeSort?: boolean
  } = {}
): URLSearchParams {
  const params = new URLSearchParams()
  const includeTimeRange = options.includeTimeRange ?? true
  if (shouldIncludeTimeRange(filters, includeTimeRange)) {
    params.append('start_timestamp', String(filters.start_timestamp))
    params.append('end_timestamp', String(filters.end_timestamp))
  }
  params.append('granularity', filters.granularity)
  params.append('limit', String(filters.limit))
  params.append('offset', String(filters.offset))
  params.append('sort_order', filters.sort_order)
  if (filters.snapshot_at !== undefined) {
    params.append('snapshot_at', String(filters.snapshot_at))
  }
  if (filters.currency !== undefined)
    params.append('currency', filters.currency)
  params.append('excluded_mode', filters.excluded_mode)
  params.append('active_only', String(filters.active_only))
  if (filters.inviter_id !== undefined) {
    params.append('inviter_id', String(filters.inviter_id))
  }
  if (filters.invitee_id !== undefined) {
    params.append('invitee_id', String(filters.invitee_id))
  }
  if (
    options.includeSubscriptionID === true &&
    filters.subscription_id !== undefined
  ) {
    params.append('subscription_id', String(filters.subscription_id))
  }
  appendArray(params, 'plan_ids', filters.plan_ids)
  appendArray(params, 'user_ids', filters.user_ids)
  appendArray(params, 'token_ids', filters.token_ids)
  appendArray(params, 'channel_ids', filters.channel_ids)
  appendArray(params, 'sources', filters.sources)
  appendArray(params, 'statuses', filters.statuses)
  appendArray(params, 'subscription_statuses', filters.subscription_statuses)
  appendArray(params, 'user_statuses', filters.user_statuses)
  appendArray(params, 'log_statuses', filters.log_statuses)
  appendArray(params, 'grant_reasons', filters.grant_reasons)
  appendArray(params, 'business_codes', filters.business_codes)
  if (options.includeUsage === true) {
    params.append('group_by', filters.group_by)
    params.append('metric', filters.metric)
    params.append('plan_attribution', filters.plan_attribution)
    params.append('top_n', String(filters.top_n))
  }
  if (
    options.includeSort === true &&
    filters.sort_by !== undefined &&
    !(
      filters.currency === undefined &&
      adminAnalyticsMoneySorts.has(filters.sort_by)
    )
  ) {
    params.append('sort_by', filters.sort_by)
  }
  return params
}
