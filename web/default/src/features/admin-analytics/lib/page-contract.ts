import { adminAnalyticsApi } from '../api'
import { buildAdminAnalyticsApiParams } from './filters'
import { ADMIN_ANALYTICS_TABS } from '../constants'
import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsPanelResponse,
  AdminAnalyticsTab,
  ApiResponse,
} from '../types'

export interface AdminAnalyticsRequestDescriptor {
  id: string
  enabled: boolean
  queryKey: readonly unknown[]
  includeTimeRange: boolean
  includeSubscriptionID: boolean
  includeUsage: boolean
  includeSort: boolean
  buildParams: (filters: AdminAnalyticsCanonicalFilters) => URLSearchParams
}

const singleEndpointByTab: Record<
  Exclude<
    AdminAnalyticsTab,
    'usage' | 'paid-subscription-value' | 'invitation-paid-subscriptions'
  >,
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

type DescriptorOptions = {
  enabled?: boolean
  includeTimeRange?: boolean
  includeSubscriptionID?: boolean
  includeUsage?: boolean
  includeSort?: boolean
}

function descriptor(
  filters: AdminAnalyticsCanonicalFilters,
  id: string,
  options: DescriptorOptions = {}
): AdminAnalyticsRequestDescriptor {
  const includeTimeRange = options.includeTimeRange ?? true
  const includeSubscriptionID = options.includeSubscriptionID ?? false
  const includeUsage = options.includeUsage ?? false
  const includeSort = options.includeSort ?? false
  return {
    id,
    enabled: options.enabled ?? true,
    queryKey: ['admin-analytics', filters.tab, id, filters] as const,
    includeTimeRange,
    includeSubscriptionID,
    includeUsage,
    includeSort,
    buildParams: (nextFilters) =>
      buildAdminAnalyticsApiParams(nextFilters, {
        includeTimeRange,
        includeSubscriptionID,
        includeUsage,
        includeSort,
      }),
  }
}

export function adminAnalyticsTabLabelKey(tab: AdminAnalyticsTab): string {
  return (
    ADMIN_ANALYTICS_TABS.find((item) => item.id === tab)?.labelKey ??
    'adminAnalytics.tabs.overview'
  )
}

export function buildAdminAnalyticsRequestDescriptors(
  filters: AdminAnalyticsCanonicalFilters
): AdminAnalyticsRequestDescriptor[] {
  if (filters.tab === 'usage') {
    return [
      descriptor(filters, 'usage-consumption/summary', {
        includeUsage: true,
        includeSort: true,
      }),
      descriptor(filters, 'usage-consumption/timeseries', {
        includeUsage: true,
      }),
      descriptor(filters, 'usage-consumption/breakdown', {
        includeUsage: true,
        includeSort: true,
      }),
    ]
  }
  if (filters.tab === 'paid-subscription-value') {
    const hasSnapshot = filters.snapshot_at !== undefined
    return [
      descriptor(filters, 'paid-subscription-value/summary'),
      descriptor(filters, 'paid-subscription-value/users', {
        enabled: hasSnapshot,
        includeSort: true,
      }),
      descriptor(filters, 'paid-subscription-value/subscriptions', {
        enabled: hasSnapshot,
        includeSubscriptionID: true,
        includeSort: true,
      }),
      descriptor(filters, 'paid-subscription-value/breakdown/plans', {
        enabled: hasSnapshot,
        includeSort: true,
      }),
      descriptor(filters, 'paid-subscription-value/breakdown/sources', {
        enabled: hasSnapshot,
        includeSort: true,
      }),
    ]
  }
  if (filters.tab === 'invitation-paid-subscriptions') {
    const hasSnapshot = filters.snapshot_at !== undefined
    return [
      descriptor(filters, 'invitation-paid-subscriptions/summary'),
      descriptor(filters, 'invitation-paid-subscriptions/inviters', {
        enabled: hasSnapshot,
        includeSort: true,
      }),
      descriptor(filters, 'invitation-paid-subscriptions/invitees', {
        enabled: hasSnapshot,
        includeSort: true,
      }),
      descriptor(filters, 'invitation-paid-subscriptions/subscriptions', {
        enabled: hasSnapshot,
        includeSubscriptionID: true,
        includeSort: true,
      }),
    ]
  }
  const endpoint = singleEndpointByTab[filters.tab]
  return [descriptor(filters, endpoint, { includeSort: endpoint !== 'overview' })]
}

type UnknownPanelResponse = ApiResponse<AdminAnalyticsPanelResponse<unknown>>

export async function fetchAdminAnalyticsDescriptor(
  descriptor: AdminAnalyticsRequestDescriptor,
  filters: AdminAnalyticsCanonicalFilters
): Promise<UnknownPanelResponse> {
  switch (descriptor.id) {
    case 'overview':
      return adminAnalyticsApi.overview(filters) as Promise<UnknownPanelResponse>
    case 'plan-distribution':
      return adminAnalyticsApi.plans(filters) as Promise<UnknownPanelResponse>
    case 'quota-distribution':
      return adminAnalyticsApi.quota(filters) as Promise<UnknownPanelResponse>
    case 'user-lifecycle':
      return adminAnalyticsApi.users(filters) as Promise<UnknownPanelResponse>
    case 'subscription-conversion':
      return adminAnalyticsApi.conversion(filters) as Promise<UnknownPanelResponse>
    case 'invitation-rewards':
      return adminAnalyticsApi.invitations(filters) as Promise<UnknownPanelResponse>
    case 'usage-consumption/summary':
      return adminAnalyticsApi.usageSummary(filters) as Promise<UnknownPanelResponse>
    case 'usage-consumption/timeseries':
      return adminAnalyticsApi.usageTimeseries(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'usage-consumption/breakdown':
      return adminAnalyticsApi.usageBreakdown(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'risks':
      return adminAnalyticsApi.risks(filters) as Promise<UnknownPanelResponse>
    case 'paid-subscription-value/summary':
      return adminAnalyticsApi.paidSubscriptionValueSummary(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'paid-subscription-value/users':
      return adminAnalyticsApi.paidSubscriptionValueUsers(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'paid-subscription-value/subscriptions':
      return adminAnalyticsApi.paidSubscriptionValueSubscriptions(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'paid-subscription-value/breakdown/plans':
      return adminAnalyticsApi.paidSubscriptionValuePlans(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'paid-subscription-value/breakdown/sources':
      return adminAnalyticsApi.paidSubscriptionValueSources(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'invitation-paid-subscriptions/summary':
      return adminAnalyticsApi.invitationPaidSummary(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'invitation-paid-subscriptions/inviters':
      return adminAnalyticsApi.invitationPaidInviters(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'invitation-paid-subscriptions/invitees':
      return adminAnalyticsApi.invitationPaidInvitees(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'invitation-paid-subscriptions/subscriptions':
      return adminAnalyticsApi.invitationPaidSubscriptions(
        filters
      ) as Promise<UnknownPanelResponse>
    default:
      throw new Error(`Unknown admin analytics descriptor: ${descriptor.id}`)
  }
}

export function warningReasons(
  warnings: { reason: string }[] | undefined
): string[] {
  if (warnings === undefined) return []
  return Array.from(new Set(warnings.map((warning) => warning.reason))).sort()
}
