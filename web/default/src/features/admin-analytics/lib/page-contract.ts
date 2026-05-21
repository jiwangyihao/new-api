import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsTab,
} from '../types'

export interface AdminAnalyticsRequestDescriptor {
  id: string
  enabled: boolean
  queryKey: readonly unknown[]
}

const singleEndpointByTab: Record<
  Exclude<AdminAnalyticsTab, 'usage'>,
  string
> = {
  overview: 'overview',
  plans: 'plan-distribution',
  quota: 'quota-distribution',
  users: 'user-lifecycle',
  conversion: 'subscription-conversion',
  invitations: 'invitation-rewards',
  risks: 'risks',
}

export function buildAdminAnalyticsRequestDescriptors(
  filters: AdminAnalyticsCanonicalFilters
): AdminAnalyticsRequestDescriptor[] {
  if (filters.tab === 'usage') {
    return ['summary', 'timeseries', 'breakdown'].map((id) => ({
      id: `usage-consumption/${id}`,
      enabled: true,
      queryKey: ['admin-analytics', 'usage', id, filters] as const,
    }))
  }
  const endpoint = singleEndpointByTab[filters.tab]
  return [
    {
      id: endpoint,
      enabled: true,
      queryKey: ['admin-analytics', filters.tab, endpoint, filters] as const,
    },
  ]
}

export function warningReasons(
  warnings: { reason: string }[] | undefined
): string[] {
  if (warnings === undefined) return []
  return Array.from(new Set(warnings.map((warning) => warning.reason))).sort()
}
