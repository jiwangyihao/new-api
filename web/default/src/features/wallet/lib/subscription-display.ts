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
import type { UserSubscriptionRecord } from '../../subscriptions/types'

function getNonBlank(value: string | undefined): string {
  return value?.trim() || ''
}

export function getSubscriptionDisplayLabel(
  record: UserSubscriptionRecord,
  planTitleMap: ReadonlyMap<number, string>,
  subscriptionLabel: string
): string {
  const subscription = record.subscription
  const fallbackLabel = `${subscriptionLabel} #${subscription.id}`
  const title =
    getNonBlank(record.plan?.title) ||
    getNonBlank(record.plan_title) ||
    getNonBlank(planTitleMap.get(subscription.plan_id))
  if (
    title &&
    (record.plan?.is_trial ||
      subscription.grant_reason === 'trial_code' ||
      subscription.grant_reason === 'invite_trial')
  ) {
    return title
  }

  if (!title) {
    return fallbackLabel
  }

  return `${title} · ${fallbackLabel}`
}
