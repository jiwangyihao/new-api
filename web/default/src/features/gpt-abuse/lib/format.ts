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
import { formatTimestampToDate } from '@/lib/format'

export const GPT_ABUSE_RAW_WARNING_MAX_LENGTH = 1000
export const GPT_ABUSE_EMPTY_DISPLAY = '—'

export const GPT_ABUSE_RAW_WARNING_SUMMARY_LENGTH = 240

export function formatGPTAbuseTimestamp(value: number | null | undefined): string {
  if (value === null || value === undefined || value <= 0) return GPT_ABUSE_EMPTY_DISPLAY
  return formatTimestampToDate(value)
}

export function formatGPTAbuseNumber(value: number | null | undefined): string {
  if (value === null || value === undefined) return GPT_ABUSE_EMPTY_DISPLAY
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value)
}

export function formatGPTAbuseChannel(
  channelId: number | null | undefined,
  channelName: string | null | undefined
): string {
  const name = channelName?.trim() ?? ''
  if (name !== '' && channelId && channelId > 0) return `${name} (#${channelId})`
  if (name !== '') return name
  if (channelId && channelId > 0) return `#${channelId}`
  return GPT_ABUSE_EMPTY_DISPLAY
}

export function formatFingerprintPrefix(value: string | null | undefined): string {
  const prefix = value?.trim() ?? ''
  return prefix === '' ? GPT_ABUSE_EMPTY_DISPLAY : prefix
}

export function formatBooleanBadge(value: boolean): string {
  return value ? 'gptAbuse.common.yes' : 'gptAbuse.common.no'
}

export function formatRawWarning(value: unknown): string {
  return truncateText(rawWarningToString(value), GPT_ABUSE_RAW_WARNING_MAX_LENGTH)
}

export function formatRawWarningSummary(value: unknown): string {
  return truncateText(rawWarningToString(value), GPT_ABUSE_RAW_WARNING_SUMMARY_LENGTH)
}

export function extractRawWarning(extra: unknown): unknown {
  if (!isRecord(extra)) return undefined
  const rawError = extra.raw_error
  if (rawError !== undefined) return rawError
  const upstreamWarning = extra.upstream_warning
  if (isRecord(upstreamWarning) && upstreamWarning.raw_error !== undefined) {
    return upstreamWarning.raw_error
  }
  return upstreamWarning
}

export function truncateText(value: string, maxLength: number): string {
  if (value.length <= maxLength) return value
  if (maxLength <= 1) return value.slice(0, maxLength)
  return `${value.slice(0, maxLength - 1)}…`
}

function rawWarningToString(value: unknown): string {
  if (value === undefined || value === null) return GPT_ABUSE_EMPTY_DISPLAY
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
