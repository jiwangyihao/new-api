import { fetchAdminAnalyticsPath } from '../api'
import { ADMIN_ANALYTICS_TABS } from '../constants'
import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsPanelResponse,
  AdminAnalyticsTab,
  ApiResponse,
} from '../types'
import { buildAdminAnalyticsApiParams } from './filters'

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

const sortableSingleEndpoints = new Set([
  'plan-distribution',
  'quota-distribution',
  'invitation-rewards',
  'risks',
])

type DescriptorOptions = {
  enabled?: boolean
  includeTimeRange?: boolean
  includeSubscriptionID?: boolean
  includeUsage?: boolean
  includeSort?: boolean
  sortBy?: string | null
}

const paidSubscriptionSortByByDescriptor: Record<
  string,
  ReadonlySet<string>
> = {
  'paid-subscription-value/users': new Set([
    'recognized_remaining_value',
    'active_paid_plan_count',
    'earliest_end_time',
    'user_id',
  ]),
  'paid-subscription-value/subscriptions': new Set([
    'recognized_remaining_value',
    'end_time',
    'start_time',
    'plan_price',
    'subscription_id',
  ]),
  'paid-subscription-value/breakdown/plans': new Set([
    'recognized_remaining_value',
    'subscription_count',
    'user_count',
    'plan_id',
  ]),
  'paid-subscription-value/breakdown/sources': new Set([
    'recognized_remaining_value',
    'subscription_count',
    'user_count',
    'source',
    'grant_reason',
  ]),
  'invitation-paid-subscriptions/inviters': new Set([
    'recognized_invitation_paid_amount',
    'active_invitation_paid_amount',
    'active_invitation_remaining_value',
    'paid_invitee_count',
    'active_paid_invitee_count',
    'inviter_user_id',
  ]),
  'invitation-paid-subscriptions/invitees': new Set([
    'recognized_paid_amount',
    'active_remaining_value',
    'paid_subscription_snapshot_count',
    'registered_at',
    'invitee_user_id',
  ]),
  'invitation-paid-subscriptions/subscriptions': new Set([
    'recognized_paid_amount',
    'recognized_remaining_value',
    'start_time',
    'end_time',
    'plan_price',
    'subscription_id',
  ]),
}

function sortByForDescriptor(
  descriptorID: string,
  sortBy: string | undefined
): string | null {
  if (sortBy === undefined) return null
  const allowed = paidSubscriptionSortByByDescriptor[descriptorID]
  if (allowed === undefined) return sortBy
  return allowed.has(sortBy) ? sortBy : null
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
    buildParams: (nextFilters) => {
      const sortBy =
        options.sortBy === undefined ? nextFilters.sort_by : options.sortBy
      return buildAdminAnalyticsApiParams(
        sortBy === null
          ? { ...nextFilters, sort_by: undefined }
          : { ...nextFilters, sort_by: sortBy },
        {
          includeTimeRange,
          includeSubscriptionID,
          includeUsage,
          includeSort: includeSort && sortBy !== null,
        }
      )
    },
  }
}

export function adminAnalyticsTabLabelKey(tab: AdminAnalyticsTab): string {
  return (
    ADMIN_ANALYTICS_TABS.find((item) => item.id === tab)?.labelKey ??
    'adminAnalytics.tabs.overview'
  )
}

const adminAnalyticsDescriptorIDs = new Set([
  ...Object.values(singleEndpointByTab),
  'usage-consumption/summary',
  'usage-consumption/timeseries',
  'usage-consumption/breakdown',
  'paid-subscription-value/summary',
  'paid-subscription-value/users',
  'paid-subscription-value/subscriptions',
  'paid-subscription-value/breakdown/plans',
  'paid-subscription-value/breakdown/sources',
  'invitation-paid-subscriptions/summary',
  'invitation-paid-subscriptions/inviters',
  'invitation-paid-subscriptions/invitees',
  'invitation-paid-subscriptions/subscriptions',
])
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
    const sortFor = (id: string) => sortByForDescriptor(id, filters.sort_by)
    return [
      descriptor(filters, 'paid-subscription-value/summary'),
      descriptor(filters, 'paid-subscription-value/users', {
        enabled: hasSnapshot,
        includeSort: true,
        sortBy: sortFor('paid-subscription-value/users'),
      }),
      descriptor(filters, 'paid-subscription-value/subscriptions', {
        enabled: hasSnapshot,
        includeSubscriptionID: true,
        includeSort: true,
        sortBy: sortFor('paid-subscription-value/subscriptions'),
      }),
      descriptor(filters, 'paid-subscription-value/breakdown/plans', {
        enabled: hasSnapshot,
        includeSort: true,
        sortBy: sortFor('paid-subscription-value/breakdown/plans'),
      }),
      descriptor(filters, 'paid-subscription-value/breakdown/sources', {
        enabled: hasSnapshot,
        includeSort: true,
        sortBy: sortFor('paid-subscription-value/breakdown/sources'),
      }),
    ]
  }
  if (filters.tab === 'invitation-paid-subscriptions') {
    const hasSnapshot = filters.snapshot_at !== undefined
    const sortFor = (id: string) => sortByForDescriptor(id, filters.sort_by)
    return [
      descriptor(filters, 'invitation-paid-subscriptions/summary'),
      descriptor(filters, 'invitation-paid-subscriptions/inviters', {
        enabled: hasSnapshot,
        includeSort: true,
        sortBy: sortFor('invitation-paid-subscriptions/inviters'),
      }),
      descriptor(filters, 'invitation-paid-subscriptions/invitees', {
        enabled: hasSnapshot,
        includeSort: true,
        sortBy: sortFor('invitation-paid-subscriptions/invitees'),
      }),
      descriptor(filters, 'invitation-paid-subscriptions/subscriptions', {
        enabled: hasSnapshot,
        includeSubscriptionID: true,
        includeSort: true,
        sortBy: sortFor('invitation-paid-subscriptions/subscriptions'),
      }),
    ]
  }
  const endpoint = singleEndpointByTab[filters.tab]
  return [
    descriptor(filters, endpoint, {
      includeSort: sortableSingleEndpoints.has(endpoint),
    }),
  ]
}

type UnknownPanelResponse = ApiResponse<AdminAnalyticsPanelResponse<unknown>>

export async function fetchAdminAnalyticsDescriptor(
  descriptor: AdminAnalyticsRequestDescriptor,
  filters: AdminAnalyticsCanonicalFilters
): Promise<UnknownPanelResponse> {
  if (!adminAnalyticsDescriptorIDs.has(descriptor.id)) {
    throw new Error(`Unknown admin analytics descriptor: ${descriptor.id}`)
  }
  return fetchAdminAnalyticsPath(descriptor.id, descriptor.buildParams(filters))
}

export function warningReasons(
  warnings: { reason: string }[] | undefined
): string[] {
  if (warnings === undefined) return []
  return Array.from(new Set(warnings.map((warning) => warning.reason))).sort()
}
