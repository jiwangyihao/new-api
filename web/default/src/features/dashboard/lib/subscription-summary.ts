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
  runwayDays: number | null
  statusLabelKey: string
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

export function getSubscriptionRunwayDays(
  summary: SelfSubscriptionSummary | undefined,
  recentTokens: number
): number | null {
  if (!summary || summary.active_count <= 0 || summary.token_unlimited) {
    return null
  }
  const remaining = normalizeAmount(summary.token_remaining)
  const recent = normalizeAmount(recentTokens)
  if (remaining <= 0 || recent <= 0) return null
  const days = remaining / recent
  if (!Number.isFinite(days)) return null
  return days
}

export function buildSubscriptionSummaryView(
  summary: SelfSubscriptionSummary | undefined,
  recentTokens = 0
): SubscriptionSummaryView {
  if (!summary || summary.active_count <= 0) {
    return {
      remainingLabel: 'Subscription required',
      usedLabel: '0',
      limitLabel: '0',
      healthLevel: 'critical',
      runwayDays: null,
      statusLabelKey: 'Subscription required',
    }
  }

  const used = normalizeAmount(summary.token_used)

  if (summary.token_unlimited) {
    return {
      remainingLabel: 'Unlimited',
      usedLabel: formatSubscriptionTokenAmount(used),
      limitLabel: 'Unlimited',
      healthLevel: 'healthy',
      runwayDays: null,
      statusLabelKey: 'Healthy',
    }
  }

  const remaining = normalizeAmount(summary.token_remaining)
  const runwayDays = getSubscriptionRunwayDays(summary, recentTokens)
  let healthLevel: SubscriptionSummaryHealthLevel = 'healthy'
  let statusLabelKey = 'Healthy'

  if (remaining <= 0) {
    healthLevel = 'critical'
    statusLabelKey = 'Tokens depleted'
  } else if (runwayDays !== null && runwayDays < 3) {
    healthLevel = 'caution'
    statusLabelKey = 'Low token balance'
  }

  return {
    remainingLabel: formatSubscriptionTokenAmount(remaining),
    usedLabel: formatSubscriptionTokenAmount(used),
    limitLabel: formatSubscriptionTokenAmount(summary.token_limit),
    healthLevel,
    runwayDays,
    statusLabelKey,
  }
}
