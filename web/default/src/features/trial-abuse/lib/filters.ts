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
  TrialAbuseRiskReason,
  TrialAbuseSummaryParams,
  TrialAbuseWarningReason,
} from '../types'

const DAY_SECONDS = 24 * 60 * 60
export const TRIAL_ABUSE_DEFAULT_WINDOW_SECONDS = 14 * DAY_SECONDS
export const TRIAL_ABUSE_MAX_WINDOW_SECONDS = 90 * DAY_SECONDS
export const TRIAL_ABUSE_DEFAULT_MIN_CONSUME_COUNT = 500
export const TRIAL_ABUSE_DEFAULT_MIN_CLUSTER_SIZE = 2
export const TRIAL_ABUSE_DEFAULT_RISK_LIMIT = 50
export const TRIAL_ABUSE_DEFAULT_GROUP_LIMIT = 20
export const TRIAL_ABUSE_MAX_RISK_LIMIT = 200
export const TRIAL_ABUSE_MAX_GROUP_LIMIT = 100

export const TRIAL_ABUSE_RISK_REASON_KEYS = [
  'sameRegistrationIpCluster',
  'sameRegistrationIpSelfInviteChain',
  'inviterLowPaidConversion',
  'managedInviterDisplayOnly',
  'registrationIpUnavailable',
  'logUnavailable',
  'candidateLimitExceeded',
  'logLimitExceeded',
] as const satisfies readonly TrialAbuseRiskReason[]

export const TRIAL_ABUSE_WARNING_REASONS = [
  'log_unavailable',
  'registration_ip_unavailable',
  'candidate_limit_exceeded',
  'log_limit_exceeded',
] as const satisfies readonly TrialAbuseWarningReason[]

export type TrialAbuseDraftFilters = {
  trialEndStart: number
  trialEndEnd: number
  registeredStart: number
  registeredEnd: number
  minConsumeCount: number
  minClusterSize: number
  riskLimit: number
  groupLimit: number
}

export type TrialAbuseFilterValidation = {
  valid: boolean
  errors: string[]
}

export function createDefaultTrialAbuseDraftFilters(
  now = currentUnixSeconds()
): TrialAbuseDraftFilters {
  return {
    trialEndStart: now - TRIAL_ABUSE_DEFAULT_WINDOW_SECONDS,
    trialEndEnd: now,
    registeredStart: 0,
    registeredEnd: 0,
    minConsumeCount: TRIAL_ABUSE_DEFAULT_MIN_CONSUME_COUNT,
    minClusterSize: TRIAL_ABUSE_DEFAULT_MIN_CLUSTER_SIZE,
    riskLimit: TRIAL_ABUSE_DEFAULT_RISK_LIMIT,
    groupLimit: TRIAL_ABUSE_DEFAULT_GROUP_LIMIT,
  }
}

export function validateTrialAbuseDraftFilters(
  draft: TrialAbuseDraftFilters
): TrialAbuseFilterValidation {
  const errors: string[] = []

  if (draft.trialEndStart <= 0 || draft.trialEndEnd <= 0) {
    errors.push('trialAbuse.validation.trialEndRangeRequired')
  } else if (draft.trialEndStart > draft.trialEndEnd) {
    errors.push('trialAbuse.validation.trialEndRangeInvalid')
  } else if (
    draft.trialEndEnd - draft.trialEndStart >
    TRIAL_ABUSE_MAX_WINDOW_SECONDS
  ) {
    errors.push('trialAbuse.validation.trialEndRangeTooLarge')
  }

  if (
    (draft.registeredStart > 0 || draft.registeredEnd > 0) &&
    (draft.registeredStart <= 0 ||
      draft.registeredEnd <= 0 ||
      draft.registeredStart > draft.registeredEnd)
  ) {
    errors.push('trialAbuse.validation.registeredRangeInvalid')
  }

  if (draft.minConsumeCount < 1 || draft.minConsumeCount > 100000) {
    errors.push('trialAbuse.validation.minConsumeCountRange')
  }

  if (draft.minClusterSize < 2 || draft.minClusterSize > 100) {
    errors.push('trialAbuse.validation.minClusterSizeRange')
  }

  if (draft.riskLimit < 1 || draft.riskLimit > TRIAL_ABUSE_MAX_RISK_LIMIT) {
    errors.push('trialAbuse.validation.riskLimitRange')
  }

  if (draft.groupLimit < 1 || draft.groupLimit > TRIAL_ABUSE_MAX_GROUP_LIMIT) {
    errors.push('trialAbuse.validation.groupLimitRange')
  }

  return { valid: errors.length === 0, errors }
}

export function buildTrialAbuseSummaryParams(
  draft: TrialAbuseDraftFilters
): TrialAbuseSummaryParams {
  const params: TrialAbuseSummaryParams = {
    trial_end_start: Math.trunc(draft.trialEndStart),
    trial_end_end: Math.trunc(draft.trialEndEnd),
    min_consume_count: clampInteger(draft.minConsumeCount, 1, 100000),
    min_cluster_size: clampInteger(draft.minClusterSize, 2, 100),
    risk_limit: clampInteger(draft.riskLimit, 1, TRIAL_ABUSE_MAX_RISK_LIMIT),
    group_limit: clampInteger(draft.groupLimit, 1, TRIAL_ABUSE_MAX_GROUP_LIMIT),
  }

  if (draft.registeredStart > 0) {
    params.registered_start = Math.trunc(draft.registeredStart)
  }
  if (draft.registeredEnd > 0) {
    params.registered_end = Math.trunc(draft.registeredEnd)
  }

  return params
}

export function updateTrialAbuseDraftFilter<K extends keyof TrialAbuseDraftFilters>(
  draft: TrialAbuseDraftFilters,
  key: K,
  value: TrialAbuseDraftFilters[K]
): TrialAbuseDraftFilters {
  return { ...draft, [key]: value }
}

export function trialAbuseRiskReasonI18nKey(
  key: TrialAbuseRiskReason
): string {
  return `trialAbuse.riskReason.${key}`
}

export function trialAbuseWarningReasonI18nKey(
  key: TrialAbuseWarningReason
): string {
  return `trialAbuse.warningReason.${key}`
}

export function unixSecondsToDateTimeInput(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return ''
  const date = new Date(value * 1000)
  const year = date.getFullYear()
  const month = padDatePart(date.getMonth() + 1)
  const day = padDatePart(date.getDate())
  const hours = padDatePart(date.getHours())
  const minutes = padDatePart(date.getMinutes())
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

export function dateTimeInputToUnixSeconds(value: string): number {
  if (value.trim() === '') return 0
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return 0
  return Math.floor(timestamp / 1000)
}

export function currentUnixSeconds(): number {
  return Math.floor(Date.now() / 1000)
}

function clampInteger(value: number, min: number, max: number): number {
  if (!Number.isFinite(value)) return min
  return Math.min(Math.max(Math.trunc(value), min), max)
}

function padDatePart(value: number): string {
  return String(value).padStart(2, '0')
}
