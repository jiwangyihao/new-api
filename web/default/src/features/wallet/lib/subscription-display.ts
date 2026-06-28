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
  PlanChannelCreditEquivalent,
  PlanChannelTokenEquivalent,
  SubscriptionChannelCreditEquivalent,
  SubscriptionChannelTokenEquivalent,
  UserSubscriptionRecord,
} from '../../subscriptions/types'

function getNonBlank(value: string | undefined): string {
  return value?.trim() || ''
}

type TranslationFn = (key: string, options?: Record<string, unknown>) => string

type PlanEquivalent = PlanChannelCreditEquivalent | PlanChannelTokenEquivalent
type SubscriptionEquivalent =
  | SubscriptionChannelCreditEquivalent
  | SubscriptionChannelTokenEquivalent

type AnyEquivalent = PlanEquivalent | SubscriptionEquivalent

export interface VisibleChannelEquivalents<T> {
  items: T[]
  hiddenCount: number
}

function getChannelTypeLabel(item: AnyEquivalent, t: TranslationFn): string {
  // 钱包折算现在按「渠道分组」聚合，后端在 channel_type_name 写入分组名（channel_type 写入分组 id）。
  // 分组名是权威展示标签，优先使用；仅在缺失时回落到静态渠道类型名或 #id。
  if (item.channel_type_name) {
    return item.channel_type_name
  }
  const labelKey = CHANNEL_TYPE_OPTIONS.find(
    (option) => option.value === item.channel_type
  )?.label
  return labelKey ? t(labelKey) : `#${item.channel_type}`
}

function formatAboutTokens(value: number, t: TranslationFn): string {
  return `${t('about')} ${formatFiniteTokenCount(value, t)}`
}

function formatRequestCount(value: number, t: TranslationFn): string {
  const requests = Number(value || 0)
  return t('{{count}} requests', { count: requests })
}

function isLegacyDefaultTokenEquivalent(item: AnyEquivalent): boolean {
  return item.kind === 'single' && 'multiplier' in item && item.multiplier === 1
}

function isDefaultUsageCreditEquivalent(item: AnyEquivalent): boolean {
  return (
    item.kind === 'usage_tokens' &&
    item.value_type === 'single' &&
    'multiplier' in item &&
    item.multiplier === 1
  )
}

function usesEstimatedTokenEquivalent(item: AnyEquivalent): boolean {
  return (
    item.kind === 'usage_tokens' || item.kind === 'single' || item.kind === 'range'
  )
}

function usesFixedRequestEquivalent(item: AnyEquivalent): boolean {
  return item.kind === 'fixed_request'
}

export function getChannelEquivalentNotes(
  items: readonly AnyEquivalent[] | undefined,
  t: TranslationFn
): string[] {
  const safeItems = items ?? []
  const notes: string[] = []
  if (safeItems.some(usesEstimatedTokenEquivalent)) {
    notes.push(
      t(
        'Estimated by current channel multiplier. Actual deduction depends on the channel used.'
      )
    )
  }
  if (safeItems.some(usesFixedRequestEquivalent)) {
    notes.push(
      t('Fixed-request channel equivalents show exact full requests available.')
    )
  }
  return notes
}

export function shouldShowChannelEquivalents(
  items: readonly AnyEquivalent[] | undefined
): boolean {
  if (!items?.length) return false
  return !items.every(
    (item) =>
      isLegacyDefaultTokenEquivalent(item) || isDefaultUsageCreditEquivalent(item)
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
  item: PlanEquivalent,
  t: TranslationFn
): string {
  const label = getChannelTypeLabel(item, t)

  if (item.kind === 'unlimited') {
    return `${label}: ${t('Unlimited tokens')}`
  }

  if (item.kind === 'usage_tokens') {
    if (item.value_type === 'range') {
      return `${label}: ${formatAboutTokens(
        item.equivalent_token_limit_min,
        t
      )} - ${formatFiniteTokenCount(item.equivalent_token_limit_max, t)}`
    }
    return `${label}: ${formatAboutTokens(item.equivalent_token_limit, t)}`
  }

  if (item.kind === 'fixed_request') {
    if (item.value_type === 'range') {
      return `${label}: ${formatRequestCount(
        item.equivalent_request_limit_min,
        t
      )} - ${t('{{count}} requests', {
        count: item.equivalent_request_limit_max,
      })}`
    }
    return `${label}: ${formatRequestCount(
      item.equivalent_request_limit,
      t
    )}`
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
  item: SubscriptionEquivalent,
  t: TranslationFn
): string {
  const label = getChannelTypeLabel(item, t)

  if (item.kind === 'unlimited') {
    return `${label}: ${t('Unlimited tokens')}`
  }

  if (item.kind === 'usage_tokens') {
    if (item.value_type === 'range') {
      return `${label}: ${formatAboutTokens(
        item.equivalent_token_remaining_min,
        t
      )} - ${formatFiniteTokenCount(item.equivalent_token_remaining_max, t)}`
    }
    return `${label}: ${formatAboutTokens(item.equivalent_token_remaining, t)}`
  }

  if (item.kind === 'fixed_request') {
    if (item.value_type === 'range') {
      return `${label}: ${formatRequestCount(
        item.equivalent_request_remaining_min,
        t
      )} - ${t('{{count}} requests', {
        count: item.equivalent_request_remaining_max,
      })}`
    }
    return `${label}: ${formatRequestCount(
      item.equivalent_request_remaining,
      t
    )}`
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
