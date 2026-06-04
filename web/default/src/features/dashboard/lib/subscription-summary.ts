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
import type { SelfSubscriptionSummary } from '@/features/subscriptions/types'

export type SubscriptionSummaryHealthLevel = 'healthy' | 'caution' | 'critical'

export interface SubscriptionSummaryView {
  remainingLabel: string
  usedLabel: string
  limitLabel: string
  healthLevel: SubscriptionSummaryHealthLevel
  timeLabelKey: string
  timeTimestamp: number | null
  statusLabelKey: string
  gptAbuseWarningLabel: string
  gptAbuseWarningRemainingLabel: string
  gptAbuseSuspendedUntil: number | null
  gptAbuseLimitEnabled: boolean
  gptAbuseStatusLabelKey: string
  gptAbuseStatusDescriptionKey: string
  gptAbuseStatusTimestamp: number | null
}

function normalizeAmount(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return 0
  }
  return Math.floor(value)
}

export function formatSubscriptionTokenAmount(value: number): string {
  const amount = normalizeAmount(value)
  if (amount < 1000) return amount.toString()
  if (amount < 1_000_000) {
    const thousands = amount / 1000
    return `${thousands < 10 ? thousands.toFixed(1) : Math.round(thousands)}K`
  }
  const millions = amount / 1_000_000
  return `${millions < 10 ? millions.toFixed(2) : millions.toFixed(1)}M`
}

interface GPTAbuseStatusView {
  gptAbuseStatusLabelKey: string
  gptAbuseStatusDescriptionKey: string
  gptAbuseStatusTimestamp: number | null
}

function buildGPTAbuseStatus(
  enabled: boolean,
  remaining: number,
  suspendedUntil: number | null
): GPTAbuseStatusView {
  if (!enabled) {
    return {
      gptAbuseStatusLabelKey: 'GPT safety warnings are observation only',
      gptAbuseStatusDescriptionKey:
        'Warnings are counted for visibility; service is not paused automatically.',
      gptAbuseStatusTimestamp: null,
    }
  }

  if (suspendedUntil) {
    return {
      gptAbuseStatusLabelKey: 'GPT service is paused',
      gptAbuseStatusDescriptionKey: 'GPT service resumes at {{time}}',
      gptAbuseStatusTimestamp: suspendedUntil,
    }
  }

  if (remaining <= 0) {
    return {
      gptAbuseStatusLabelKey: 'GPT safety warning limit reached',
      gptAbuseStatusDescriptionKey:
        'Service interruption is enabled; reaching the limit pauses GPT access until the next day.',
      gptAbuseStatusTimestamp: null,
    }
  }

  return {
    gptAbuseStatusLabelKey:
      'GPT safety warnings will pause service at the daily limit',
    gptAbuseStatusDescriptionKey:
      'Service interruption is enabled; reaching the limit pauses GPT access until the next day.',
    gptAbuseStatusTimestamp: null,
  }
}

export function buildSubscriptionSummaryView(
  summary: SelfSubscriptionSummary | undefined,
  _recentTokens = 0
): SubscriptionSummaryView {
  if (!summary || summary.active_count <= 0) {
    return {
      remainingLabel: 'Subscription required',
      usedLabel: '0',
      limitLabel: '0',
      healthLevel: 'critical',
      timeLabelKey: 'Subscription expires at',
      timeTimestamp: null,
      statusLabelKey: 'Subscription required',
      gptAbuseWarningLabel: '0 / 0',
      gptAbuseWarningRemainingLabel: '0',
      gptAbuseSuspendedUntil: null,
      gptAbuseLimitEnabled: false,
      gptAbuseStatusLabelKey: 'GPT safety warnings unavailable',
      gptAbuseStatusDescriptionKey:
        'Activate a subscription to see GPT safety warning status.',
      gptAbuseStatusTimestamp: null,
    }
  }

  const gptAbuseLimit = normalizeAmount(summary.gpt_abuse_warning_limit)
  const gptAbuseCount = normalizeAmount(summary.gpt_abuse_warning_count)
  const gptAbuseRemaining = normalizeAmount(summary.gpt_abuse_warning_remaining)
  const gptAbuseWarningLabel = `${gptAbuseCount} / ${gptAbuseLimit}`
  const gptAbuseWarningRemainingLabel = `${gptAbuseRemaining}`
  const gptAbuseSuspendedUntil = summary.gpt_abuse_suspended_until || null
  const gptAbuseLimitEnabled = summary.gpt_abuse_limit_enabled === true
  const gptAbuseStatus = buildGPTAbuseStatus(
    gptAbuseLimitEnabled,
    gptAbuseRemaining,
    gptAbuseSuspendedUntil
  )
  const used = normalizeAmount(summary.token_used)

  if (summary.token_unlimited) {
    return {
      remainingLabel: 'Unlimited',
      usedLabel: formatSubscriptionTokenAmount(used),
      limitLabel: 'Unlimited',
      healthLevel: 'healthy',
      timeLabelKey: summary.next_reset_time
        ? 'Subscription resets at'
        : 'Subscription expires at',
      timeTimestamp: summary.next_reset_time || summary.end_time || null,
      statusLabelKey: 'Healthy',
      gptAbuseWarningLabel,
      gptAbuseWarningRemainingLabel,
      gptAbuseSuspendedUntil,
      gptAbuseLimitEnabled,
      ...gptAbuseStatus,
    }
  }

  const remaining = normalizeAmount(summary.token_remaining)

  let healthLevel: SubscriptionSummaryHealthLevel = 'healthy'
  let statusLabelKey = 'Healthy'

  if (remaining <= 0) {
    healthLevel = 'critical'
    statusLabelKey = 'Tokens depleted'
  }

  return {
    remainingLabel: formatSubscriptionTokenAmount(remaining),
    usedLabel: formatSubscriptionTokenAmount(used),
    limitLabel: formatSubscriptionTokenAmount(summary.token_limit),
    healthLevel,
    timeLabelKey: summary.next_reset_time
      ? 'Subscription resets at'
      : 'Subscription expires at',
    timeTimestamp: summary.next_reset_time || summary.end_time || null,
    statusLabelKey,
    gptAbuseWarningLabel,
    gptAbuseWarningRemainingLabel,
    gptAbuseSuspendedUntil,
    gptAbuseLimitEnabled,
    ...gptAbuseStatus,
  }
}
