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
} from './types'

function adminAnalyticsUrl(
  path: string,
  filters: AdminAnalyticsCanonicalFilters,
  options: { includeUsage?: boolean; includeSort?: boolean } = {}
): string {
  const params = buildAdminAnalyticsApiParams(filters, options)
  return `/api/admin-analytics/${path}?${params.toString()}`
}

async function getAdminAnalytics<T>(
  path: string,
  filters: AdminAnalyticsCanonicalFilters,
  options: { includeUsage?: boolean; includeSort?: boolean } = {}
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
}
