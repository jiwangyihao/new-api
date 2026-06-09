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
  GPT_ABUSE_DEFAULT_LIMIT,
  GPT_ABUSE_DETAIL_LIMIT,
  GPT_ABUSE_KIND_OPTIONS,
  GPT_ABUSE_SEVERITY_OPTIONS,
  GPT_ABUSE_SORT_BY_OPTIONS,
  GPT_ABUSE_SORT_ORDER_OPTIONS,
  GPT_ABUSE_SOURCE_OPTIONS,
  GPT_ABUSE_STATUS_OPTIONS,
} from '../constants'
import type {
  GPTAbuseCountEligibleFilter,
  GPTAbuseLogSearch,
  GPTAbuseRepeatBlockSearch,
  GPTAbuseSortBy,
  GPTAbuseSortOrder,
  GPTAbuseStatusFilter,
  GPTAbuseUserListSearch,
} from '../types'

const DAY_SECONDS = 24 * 60 * 60
const maxLimit = 100
const statusSet = new Set<string>(GPT_ABUSE_STATUS_OPTIONS)
const kindSet = new Set<string>(GPT_ABUSE_KIND_OPTIONS)
const severitySet = new Set<string>(GPT_ABUSE_SEVERITY_OPTIONS)
const sourceSet = new Set<string>(GPT_ABUSE_SOURCE_OPTIONS)
const sortBySet = new Set<string>(GPT_ABUSE_SORT_BY_OPTIONS)
const sortOrderSet = new Set<string>(GPT_ABUSE_SORT_ORDER_OPTIONS)

const looseSearchSchema = z.object({}).catchall(z.unknown())

export const gptAbuseSearchSchema: z.ZodType<GPTAbuseUserListSearch> = z
  .unknown()
  .transform((search) => buildGPTAbuseCanonicalSearch(search))

export function buildGPTAbuseCanonicalSearch(
  search: unknown,
  currentSeconds = currentUnixSeconds()
): GPTAbuseUserListSearch {
  const source = looseSearchSchema.parse(search)
  const todayStart = startOfLocalDaySeconds(currentSeconds)
  return {
    start_timestamp: parseOptionalPositiveInteger(source.start_timestamp) ?? todayStart,
    end_timestamp:
      parseOptionalPositiveInteger(source.end_timestamp) ?? todayStart + DAY_SECONDS,
    keyword: parseString(source.keyword),
    user_id: parseOptionalPositiveInteger(source.user_id),
    status: parseEnum<GPTAbuseStatusFilter>(source.status, statusSet, 'all'),
    kind: parseOptionalEnum(source.kind, kindSet),
    severity: parseOptionalEnum(source.severity, severitySet),
    source: parseOptionalEnum(source.source, sourceSet),
    limit: parseLimit(source.limit, GPT_ABUSE_DEFAULT_LIMIT),
    offset: parseOffset(source.offset),
    sort_by: parseEnum<GPTAbuseSortBy>(
      source.sort_by,
      sortBySet,
      'latest_warning_at'
    ),
    sort_order: parseEnum<GPTAbuseSortOrder>(source.sort_order, sortOrderSet, 'desc'),
  }
}

export function buildGPTAbuseLogSearch(
  userSearch: GPTAbuseUserListSearch,
  patch: Partial<GPTAbuseLogSearch> = {}
): GPTAbuseLogSearch {
  return {
    start_timestamp: userSearch.start_timestamp,
    end_timestamp: userSearch.end_timestamp,
    source: userSearch.source,
    kind: userSearch.kind,
    severity: userSearch.severity,
    count_eligible: 'all',
    limit: GPT_ABUSE_DETAIL_LIMIT,
    offset: 0,
    ...patch,
  }
}

export function buildGPTAbuseRepeatBlockSearch(
  userSearch: GPTAbuseUserListSearch,
  patch: Partial<GPTAbuseRepeatBlockSearch> = {}
): GPTAbuseRepeatBlockSearch {
  return {
    start_timestamp: userSearch.start_timestamp,
    end_timestamp: userSearch.end_timestamp,
    limit: GPT_ABUSE_DETAIL_LIMIT,
    offset: 0,
    ...patch,
  }
}

export function updateGPTAbuseSearchForFilterChange(
  current: GPTAbuseUserListSearch,
  patch: Partial<GPTAbuseUserListSearch>
): GPTAbuseUserListSearch {
  return { ...current, ...patch, offset: 0 }
}

export function updateGPTAbuseSearchForPagination(
  current: GPTAbuseUserListSearch,
  patch: Pick<Partial<GPTAbuseUserListSearch>, 'limit' | 'offset'>
): GPTAbuseUserListSearch {
  return { ...current, ...patch }
}

export function updateGPTAbuseSearchForSorting(
  current: GPTAbuseUserListSearch,
  patch: Pick<Partial<GPTAbuseUserListSearch>, 'sort_by' | 'sort_order'>
): GPTAbuseUserListSearch {
  return { ...current, ...patch }
}

export function buildGPTAbuseApiParams(
  search: GPTAbuseUserListSearch
): URLSearchParams {
  const params = new URLSearchParams()
  appendOptionalNumber(params, 'start_timestamp', search.start_timestamp)
  appendOptionalNumber(params, 'end_timestamp', search.end_timestamp)
  appendOptionalString(params, 'keyword', search.keyword)
  appendOptionalNumber(params, 'user_id', search.user_id)
  appendOptionalString(params, 'status', search.status)
  appendOptionalString(params, 'kind', search.kind)
  appendOptionalString(params, 'severity', search.severity)
  appendOptionalString(params, 'source', search.source)
  params.set('limit', String(search.limit))
  params.set('offset', String(search.offset))
  params.set('sort_by', search.sort_by)
  params.set('sort_order', search.sort_order)
  return params
}

export function gptAbuseStatusLabelKey(value: GPTAbuseStatusFilter): string {
  return `gptAbuse.status.${value}`
}

export function gptAbuseSeverityLabelKey(value: string): string {
  return value === '' ? 'gptAbuse.filters.allSeverities' : `gptAbuse.severity.${value}`
}

export function gptAbuseKindLabelKey(value: string): string {
  return value === '' ? 'gptAbuse.filters.allKinds' : `gptAbuse.kind.${value}`
}

export function gptAbuseSourceLabelKey(value: string): string {
  return value === '' ? 'gptAbuse.filters.allSources' : `gptAbuse.source.${value}`
}

export function gptAbuseCountEligibleLabelKey(
  value: GPTAbuseCountEligibleFilter
): string {
  return `gptAbuse.countEligible.${value}`
}

export function gptAbuseSortByLabelKey(value: GPTAbuseSortBy): string {
  return `gptAbuse.sort.${value}`
}

export function gptAbuseSortOrderLabelKey(value: GPTAbuseSortOrder): string {
  return `gptAbuse.sortOrder.${value}`
}

export function gptAbuseSuspensionLabelKey(value: string): string {
  switch (value) {
    case 'active':
      return 'gptAbuse.suspension.active'
    case 'cleared':
      return 'gptAbuse.suspension.cleared'
    case 'expired':
      return 'gptAbuse.suspension.expired'
    default:
      return 'gptAbuse.suspension.none'
  }
}

export function currentUnixSeconds(): number {
  return Math.floor(Date.now() / 1000)
}

export function unixSecondsToDateTimeInput(value: number | undefined): string {
  if (value === undefined || value <= 0) return ''
  const date = new Date(value * 1000)
  const year = date.getFullYear()
  const month = padDatePart(date.getMonth() + 1)
  const day = padDatePart(date.getDate())
  const hour = padDatePart(date.getHours())
  const minute = padDatePart(date.getMinutes())
  return `${year}-${month}-${day}T${hour}:${minute}`
}

export function dateTimeInputToUnixSeconds(value: string): number | undefined {
  if (value.trim() === '') return undefined
  const millis = new Date(value).getTime()
  if (!Number.isFinite(millis)) return undefined
  return Math.floor(millis / 1000)
}

function startOfLocalDaySeconds(value: number): number {
  const date = new Date(value * 1000)
  date.setHours(0, 0, 0, 0)
  return Math.floor(date.getTime() / 1000)
}

function parseString(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0].trim()
  return ''
}

function parseOptionalPositiveInteger(value: unknown): number | undefined {
  const parsed = parseInteger(value)
  if (parsed === undefined || parsed <= 0) return undefined
  return parsed
}

function parseInteger(value: unknown): number | undefined {
  const raw = Array.isArray(value) ? value[0] : value
  if (raw === undefined || raw === null || raw === '') return undefined
  const parsed = Number(raw)
  if (!Number.isFinite(parsed)) return undefined
  return Math.trunc(parsed)
}

function parseOffset(value: unknown): number {
  const parsed = parseInteger(value)
  if (parsed === undefined || parsed < 0) return 0
  return parsed
}

function parseLimit(value: unknown, fallback: number): number {
  const parsed = parseInteger(value)
  if (parsed === undefined) return fallback
  return Math.min(Math.max(parsed, 1), maxLimit)
}

function parseEnum<T extends string>(
  value: unknown,
  allowed: ReadonlySet<string>,
  fallback: T
): T {
  const text = parseString(value)
  if (allowed.has(text)) return text as T
  return fallback
}

function parseOptionalEnum(value: unknown, allowed: ReadonlySet<string>): string {
  const text = parseString(value)
  if (allowed.has(text)) return text
  return ''
}

function appendOptionalNumber(
  params: URLSearchParams,
  key: string,
  value: number | undefined
): void {
  if (value !== undefined) params.set(key, String(value))
}

function appendOptionalString(
  params: URLSearchParams,
  key: string,
  value: string
): void {
  if (value !== '') params.set(key, value)
}

function padDatePart(value: number): string {
  return String(value).padStart(2, '0')
}
