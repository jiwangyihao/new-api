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
import type { AdminAnalyticsTab } from './types'

export const ADMIN_ANALYTICS_DEFAULT_RANGE_SECONDS = 30 * 24 * 60 * 60
export const ADMIN_ANALYTICS_MAX_RANGE_SECONDS = 365 * 24 * 60 * 60
export const ADMIN_ANALYTICS_DEFAULT_LIMIT = 20
export const ADMIN_ANALYTICS_MAX_LIMIT = 100

export const ADMIN_ANALYTICS_TABS: Array<{
  id: AdminAnalyticsTab
  labelKey: string
}> = [
  { id: 'overview', labelKey: 'adminAnalytics.tabs.overview' },
  { id: 'plans', labelKey: 'adminAnalytics.tabs.plans' },
  { id: 'quota', labelKey: 'adminAnalytics.tabs.quota' },
  {
    id: 'paid-subscription-value',
    labelKey: 'adminAnalytics.tabs.paidSubscriptionValue',
  },
  { id: 'users', labelKey: 'adminAnalytics.tabs.users' },
  { id: 'conversion', labelKey: 'adminAnalytics.tabs.conversion' },
  { id: 'invitations', labelKey: 'adminAnalytics.tabs.invitations' },
  {
    id: 'invitation-paid-subscriptions',
    labelKey: 'adminAnalytics.tabs.invitationPaidSubscriptions',
  },
  { id: 'usage', labelKey: 'adminAnalytics.tabs.usage' },
  { id: 'risks', labelKey: 'adminAnalytics.tabs.risks' },
]

export const ADMIN_ANALYTICS_QUOTA_BUCKET_ORDER = [
  'invalid',
  'unlimited_or_invalid',
  'zero_limit',
  '0_25',
  '25_50',
  '50_75',
  '75_90',
  '90_100',
  'over_100',
] as const
