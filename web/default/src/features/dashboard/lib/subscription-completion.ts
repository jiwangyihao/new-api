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
import type { UserSubscriptionRecord } from '@/features/subscriptions/types'

export type SubscriptionCompletion = 'none' | 'trial' | 'paid'

export function getSubscriptionCompletion(
  subscriptions: UserSubscriptionRecord[] | undefined,
  now: number = Date.now() / 1000
): SubscriptionCompletion {
  let hasTrial = false

  for (const item of subscriptions ?? []) {
    const subscription = item.subscription
    const isExpired =
      (subscription?.end_time ?? 0) > 0 && subscription.end_time < now
    if (subscription?.status !== 'active' || isExpired) continue

    const grantReason = subscription.grant_reason?.trim()
    if (grantReason === 'trial_code' || grantReason === 'invite_trial') {
      hasTrial = true
      continue
    }

    const source = subscription.source?.trim()
    if (grantReason === 'order' || (!grantReason && source === 'order')) {
      return 'paid'
    }
  }

  return hasTrial ? 'trial' : 'none'
}
