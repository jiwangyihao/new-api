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
import { CHANNEL_TYPE_OPTIONS } from '@/features/channels/constants'
import { formatFiniteTokenCount } from '@/features/subscriptions/lib/format'
import type {
  PlanChannelTokenEquivalent,
  SubscriptionChannelTokenEquivalent,
  UserSubscriptionRecord,
} from '../../subscriptions/types'

function getNonBlank(value: string | undefined): string {
  return value?.trim() || ''
}

type TranslationFn = (key: string, options?: Record<string, unknown>) => string

export interface VisibleChannelEquivalents<T> {
  items: T[]
  hiddenCount: number
}

function getChannelTypeLabel(
  item: PlanChannelTokenEquivalent | SubscriptionChannelTokenEquivalent,
  t: TranslationFn
): string {
  const labelKey = CHANNEL_TYPE_OPTIONS.find(
    (option) => option.value === item.channel_type
  )?.label

  return labelKey
    ? t(labelKey)
    : item.channel_type_name || `#${item.channel_type}`
}

function formatAboutTokens(value: number, t: TranslationFn): string {
  return `${t('about')} ${formatFiniteTokenCount(value, t)}`
}

export function shouldShowChannelEquivalents(
  items:
    | readonly PlanChannelTokenEquivalent[]
    | readonly SubscriptionChannelTokenEquivalent[]
    | undefined
): boolean {
  if (!items?.length) return false
  return !items.every(
    (item) => item.kind === 'single' && item.multiplier === 1
  )
}

export function getVisibleChannelEquivalents<T>(
  items: readonly T[] | undefined,
  limit = 3
): VisibleChannelEquivalents<T> {
  const safeItems = items ?? []
  const visibleLimit = Math.max(0, limit)

  return {
    items: safeItems.slice(0, visibleLimit),
    hiddenCount: Math.max(0, safeItems.length - visibleLimit),
  }
}

export function formatPlanChannelEquivalent(
  item: PlanChannelTokenEquivalent,
  t: TranslationFn
): string {
  const label = getChannelTypeLabel(item, t)

  if (item.kind === 'unlimited') {
    return `${label}: ${t('Unlimited tokens')}`
  }

  if (item.kind === 'range') {
    return `${label}: ${formatAboutTokens(
      item.equivalent_token_limit_min,
      t
    )} - ${formatFiniteTokenCount(item.equivalent_token_limit_max, t)}`
  }

  return `${label}: ${formatAboutTokens(item.equivalent_token_limit, t)}`
}

export function formatSubscriptionChannelEquivalent(
  item: SubscriptionChannelTokenEquivalent,
  t: TranslationFn
): string {
  const label = getChannelTypeLabel(item, t)

  if (item.kind === 'unlimited') {
    return `${label}: ${t('Unlimited tokens')}`
  }

  if (item.kind === 'range') {
    return `${label}: ${formatAboutTokens(
      item.equivalent_token_remaining_min,
      t
    )} - ${formatFiniteTokenCount(item.equivalent_token_remaining_max, t)}`
  }

  return `${label}: ${formatAboutTokens(item.equivalent_token_remaining, t)}`
}

export function getSubscriptionDisplayLabel(
  record: UserSubscriptionRecord,
  planTitleMap: ReadonlyMap<number, string>,
  subscriptionLabel = 'Subscription'
): string {
  const subscription = record.subscription
  const title =
    getNonBlank(record.plan?.title) ||
    getNonBlank(record.plan_title) ||
    getNonBlank(planTitleMap.get(subscription.plan_id))

  return title || subscriptionLabel
}
