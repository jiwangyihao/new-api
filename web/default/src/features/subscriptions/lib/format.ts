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
import dayjs from '@/lib/dayjs'
import type { SubscriptionPlan } from '../types'

type TranslationFn = (key: string, options?: Record<string, unknown>) => string

type DurationPlanLike = {
  duration_unit?: SubscriptionPlan['duration_unit'] | string
  duration_value?: number
  custom_seconds?: number
}

export function formatDuration(
  plan: DurationPlanLike,
  t: TranslationFn
): string {
  const unit = plan?.duration_unit || 'month'
  const value = plan?.duration_value || 1
  const unitLabels: Record<string, string> = {
    year: t('years'),
    month: t('months'),
    day: t('days'),
    hour: t('hours'),
    custom: t('Custom (seconds)'),
  }
  if (unit === 'custom') {
    const seconds = plan?.custom_seconds || 0
    if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('days')}`
    if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('hours')}`
    return `${seconds} ${t('seconds')}`
  }
  return `${value} ${unitLabels[unit] || unit}`
}

export function formatResetPeriod(
  plan: Partial<SubscriptionPlan>,
  t: TranslationFn
): string {
  const period = plan?.quota_reset_period || 'never'
  if (period === 'daily') return t('Daily')
  if (period === 'weekly') return t('Weekly')
  if (period === 'monthly') return t('Monthly')
  if (period === 'custom') {
    const seconds = Number(plan?.quota_reset_custom_seconds || 0)
    if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('days')}`
    if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('hours')}`
    if (seconds >= 60) return `${Math.floor(seconds / 60)} ${t('minutes')}`
    return `${seconds} ${t('seconds')}`
  }
  return t('No Reset')
}

export function formatTokenLimit(
  value: number | null | undefined,
  t: TranslationFn
): string {
  const tokens = Number(value || 0)
  if (tokens <= 0) return t('Unlimited tokens')

  if (tokens >= 1_000_000_000) {
    return `${formatCompactNumber(tokens / 1_000_000_000)}B ${t('tokens')}`
  }
  if (tokens >= 1_000_000) {
    return `${formatCompactNumber(tokens / 1_000_000)}M ${t('tokens')}`
  }
  if (tokens >= 1_000) {
    return `${formatCompactNumber(tokens / 1_000)}K ${t('tokens')}`
  }
  return `${tokens} ${t('tokens')}`
}

export function formatFiniteTokenCount(
  value: number | null | undefined,
  t: TranslationFn
): string {
  const tokens = Number(value || 0)
  if (!Number.isFinite(tokens) || tokens <= 0) return `0 ${t('tokens')}`
  return formatTokenLimit(tokens, t)
}

export function formatCreditLimit(
  value: number | null | undefined,
  t: TranslationFn
): string {
  const credits = Number(value || 0)
  if (credits <= 0) return t('Unlimited credits')

  if (credits >= 1_000_000_000) {
    return `${formatCompactNumber(credits / 1_000_000_000)}B ${t('credits')}`
  }
  if (credits >= 1_000_000) {
    return `${formatCompactNumber(credits / 1_000_000)}M ${t('credits')}`
  }
  if (credits >= 1_000) {
    return `${formatCompactNumber(credits / 1_000)}K ${t('credits')}`
  }
  return `${credits} ${t('credits')}`
}

export function formatFiniteCreditCount(
  value: number | null | undefined,
  t: TranslationFn
): string {
  const credits = Number(value || 0)
  if (!Number.isFinite(credits) || credits <= 0) return `0 ${t('credits')}`
  return formatCreditLimit(credits, t)
}

export function formatCompactCredit(value: bigint | number | string): string {
  let credits: bigint
  try {
    credits = typeof value === 'bigint' ? value : BigInt(value)
  } catch {
    return '0'
  }
  const negative = credits < 0n
  const absolute = negative ? -credits : credits
  const units = [
    { value: 1_000_000_000n, suffix: 'B' },
    { value: 1_000_000n, suffix: 'M' },
    { value: 1_000n, suffix: 'K' },
  ]
  const unit = units.find((candidate) => absolute >= candidate.value)
  if (!unit) return new Intl.NumberFormat().format(credits)
  const scaledHundredths = (absolute * 100n + unit.value / 2n) / unit.value
  const whole = scaledHundredths / 100n
  const fraction = (scaledHundredths % 100n)
    .toString()
    .padStart(2, '0')
    .replace(/0+$/, '')
  return `${negative ? '-' : ''}${whole}${fraction ? `.${fraction}` : ''}${unit.suffix}`
}

export function formatDurationSeconds(
  value: bigint | number | string,
  t: TranslationFn
): string {
  let remaining: bigint
  try {
    remaining = typeof value === 'bigint' ? value : BigInt(value)
  } catch {
    return `0 ${t('seconds')}`
  }
  if (remaining < 0n) remaining = 0n
  const units = [
    { seconds: 365n * 24n * 60n * 60n, label: 'years' },
    { seconds: 31n * 24n * 60n * 60n, label: 'months' },
    { seconds: 24n * 60n * 60n, label: 'days' },
    { seconds: 60n * 60n, label: 'hours' },
    { seconds: 60n, label: 'minutes' },
    { seconds: 1n, label: 'seconds' },
  ] as const
  const parts: string[] = []
  for (const unit of units) {
    const count = remaining / unit.seconds
    if (count > 0n) {
      parts.push(`${count} ${t(unit.label)}`)
      remaining %= unit.seconds
    }
  }
  return parts.length > 0 ? parts.join(' ') : `0 ${t('seconds')}`
}

export function formatConcurrencyLimit(
  value: number | null | undefined,
  t: TranslationFn
): string {
  const limit = Number(value || 0)
  if (limit <= 0) return t('Unlimited concurrency')
  return t('{{count}} concurrent requests', { count: limit })
}

export function formatQueueCapacity(
  value: number | null | undefined,
  t: TranslationFn
): string {
  const capacity = Number(value || 0)
  if (capacity <= 0) return t('Use global queue capacity')
  return t('{{count}} queued requests', { count: capacity })
}
export function formatPlanPrice(
  amount: number | null | undefined,
  currency: string | null | undefined = 'CNY'
): string {
  const value = Number(amount || 0)
  const code = (currency || 'CNY').trim().toUpperCase()
  const symbol = code === 'CNY' ? '¥' : code === 'USD' ? '$' : `${code} `
  return `${symbol}${value.toFixed(2)}`
}

function formatCompactNumber(value: number): string {
  if (Number.isInteger(value)) return String(value)
  return value
    .toFixed(2)
    .replace(/\.0+$/, '')
    .replace(/(\.\d*[1-9])0+$/, '$1')
}

export function formatTimestamp(ts: number): string {
  if (!ts) return '-'
  return dayjs(ts * 1000).format('YYYY-MM-DD HH:mm:ss')
}
