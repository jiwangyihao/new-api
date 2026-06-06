import { api } from '@/lib/api'
import { buildAdminAnalyticsApiParams } from './lib/filters'
import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsInvitationRewardsResponse,
  AdminAnalyticsOverviewResponse,
  AdminAnalyticsPanelResponse,
  AdminAnalyticsPlanDistributionResponse,
  AdminAnalyticsQuotaDistributionResponse,
  AdminAnalyticsRisksResponse,
  AdminAnalyticsSubscriptionConversionResponse,
  AdminAnalyticsUserLifecycleResponse,
  AdminUsageBreakdownResponse,
  AdminUsageConsumptionSummaryResponse,
  AdminUsageTimeseriesResponse,
  ApiResponse,
  InvitationPaidSubscriptionsResponse,
  PaidSubscriptionValueResponse,
} from './types'

export interface AdminAnalyticsApiParamOptions {
  includeTimeRange?: boolean
  includeSubscriptionID?: boolean
  includeUsage?: boolean
  includeSort?: boolean
}

export function adminAnalyticsUrl(
  path: string,
  filters: AdminAnalyticsCanonicalFilters,
  options: AdminAnalyticsApiParamOptions = {}
): string {
  const params = buildAdminAnalyticsApiParams(filters, options)
  return `/api/admin-analytics/${path}?${params.toString()}`
}

async function getAdminAnalytics<T>(
  path: string,
  filters: AdminAnalyticsCanonicalFilters,
  options: AdminAnalyticsApiParamOptions = {}
): Promise<ApiResponse<AdminAnalyticsPanelResponse<T>>> {
  const res = await api.get<ApiResponse<AdminAnalyticsPanelResponse<T>>>(
    adminAnalyticsUrl(path, filters, options)
  )
  return res.data
}

export const adminAnalyticsApi = {
  overview: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminAnalyticsOverviewResponse>('overview', filters),
  plans: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminAnalyticsPlanDistributionResponse>(
      'plan-distribution',
      filters,
      { includeSort: true }
    ),
  quota: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminAnalyticsQuotaDistributionResponse>(
      'quota-distribution',
      filters,
      { includeSort: true }
    ),
  users: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminAnalyticsUserLifecycleResponse>(
      'user-lifecycle',
      filters
    ),
  conversion: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminAnalyticsSubscriptionConversionResponse>(
      'subscription-conversion',
      filters
    ),
  invitations: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminAnalyticsInvitationRewardsResponse>(
      'invitation-rewards',
      filters,
      { includeSort: true }
    ),
  usageSummary: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminUsageConsumptionSummaryResponse>(
      'usage-consumption/summary',
      filters,
      { includeUsage: true, includeSort: true }
    ),
  usageTimeseries: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminUsageTimeseriesResponse>(
      'usage-consumption/timeseries',
      filters,
      { includeUsage: true }
    ),
  usageBreakdown: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminUsageBreakdownResponse>(
      'usage-consumption/breakdown',
      filters,
      { includeUsage: true, includeSort: true }
    ),
  risks: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<AdminAnalyticsRisksResponse>('risks', filters, {
      includeSort: true,
    }),
  paidSubscriptionValueSummary: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<PaidSubscriptionValueResponse>(
      'paid-subscription-value/summary',
      filters
    ),
  paidSubscriptionValueUsers: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<PaidSubscriptionValueResponse>(
      'paid-subscription-value/users',
      filters,
      { includeSort: true }
    ),
  paidSubscriptionValueSubscriptions: (
    filters: AdminAnalyticsCanonicalFilters
  ) =>
    getAdminAnalytics<PaidSubscriptionValueResponse>(
      'paid-subscription-value/subscriptions',
      filters,
      { includeSubscriptionID: true, includeSort: true }
    ),
  paidSubscriptionValuePlans: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<PaidSubscriptionValueResponse>(
      'paid-subscription-value/breakdown/plans',
      filters,
      { includeSort: true }
    ),
  paidSubscriptionValueSources: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<PaidSubscriptionValueResponse>(
      'paid-subscription-value/breakdown/sources',
      filters,
      { includeSort: true }
    ),
  invitationPaidSummary: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<InvitationPaidSubscriptionsResponse>(
      'invitation-paid-subscriptions/summary',
      filters
    ),
  invitationPaidInviters: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<InvitationPaidSubscriptionsResponse>(
      'invitation-paid-subscriptions/inviters',
      filters,
      { includeSort: true }
    ),
  invitationPaidInvitees: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<InvitationPaidSubscriptionsResponse>(
      'invitation-paid-subscriptions/invitees',
      filters,
      { includeSort: true }
    ),
  invitationPaidSubscriptions: (filters: AdminAnalyticsCanonicalFilters) =>
    getAdminAnalytics<InvitationPaidSubscriptionsResponse>(
      'invitation-paid-subscriptions/subscriptions',
      filters,
      { includeSubscriptionID: true, includeSort: true }
    ),
}
