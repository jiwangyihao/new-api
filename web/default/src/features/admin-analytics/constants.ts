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
  { id: 'users', labelKey: 'adminAnalytics.tabs.users' },
  { id: 'conversion', labelKey: 'adminAnalytics.tabs.conversion' },
  { id: 'invitations', labelKey: 'adminAnalytics.tabs.invitations' },
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
