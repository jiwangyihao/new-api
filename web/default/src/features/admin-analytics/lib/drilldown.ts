import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsDrilldownTarget,
  FrontendAdminAnalyticsDrilldownTarget,
} from '../types'

function secondsToMillis(value: number | undefined): number | undefined {
  return value === undefined ? undefined : value * 1000
}

function withAdminAnalyticsTab(
  filters: AdminAnalyticsCanonicalFilters,
  tab: AdminAnalyticsCanonicalFilters['tab']
): AdminAnalyticsCanonicalFilters {
  return { ...filters, tab }
}

function withoutSubscriptionID(
  filters: AdminAnalyticsCanonicalFilters
): AdminAnalyticsCanonicalFilters {
  const { subscription_id: _, ...rest } = filters
  return rest
}

export function buildAdminAnalyticsDrilldown(
  filters: AdminAnalyticsCanonicalFilters,
  target: AdminAnalyticsDrilldownTarget | null | undefined
): FrontendAdminAnalyticsDrilldownTarget | null {
  if (target === null || target === undefined) return null
  switch (target.kind) {
    case 'admin_users':
      return {
        to: '/users',
        search: {
          userId: target.user_id,
          userIds: target.user_ids,
          status: target.user_status,
          planId: target.plan_id,
          inviterId: target.inviter_id,
        },
      }
    case 'admin_usage_logs':
      return {
        to: '/usage-logs/$section',
        params: { section: 'common' },
        search: {
          userId: target.user_id,
          username: target.username,
          tokenId: target.token_id,
          model: target.model,
          channel: target.channel_id,
          status: target.status,
          startTime: secondsToMillis(
            target.start_timestamp ?? filters.start_timestamp
          ),
          endTime: secondsToMillis(
            target.end_timestamp ?? filters.end_timestamp
          ),
        },
      }
    case 'admin_subscriptions':
    case 'admin_invitations':
      return {
        to: '/admin-analytics',
        search: {
          ...withAdminAnalyticsTab(
            filters,
            target.kind === 'admin_subscriptions' ? 'plans' : 'invitations'
          ),
          user_ids:
            target.user_id !== undefined ? [target.user_id] : filters.user_ids,
          plan_ids:
            target.plan_id !== undefined ? [target.plan_id] : filters.plan_ids,
          inviter_id: target.inviter_id,
        },
      }
    case 'paid_subscription_value_user':
      return {
        to: '/admin-analytics',
        search: {
          ...withAdminAnalyticsTab(
            withoutSubscriptionID(filters),
            'paid-subscription-value'
          ),
          user_ids:
            target.user_id !== undefined ? [target.user_id] : filters.user_ids,
        },
      }
    case 'paid_subscription_value_subscription': {
      const search = withAdminAnalyticsTab(
        withoutSubscriptionID(filters),
        'paid-subscription-value'
      )
      if (target.subscription_id !== undefined) {
        return {
          to: '/admin-analytics',
          search: { ...search, subscription_id: target.subscription_id },
        }
      }
      return {
        to: '/admin-analytics',
        search: {
          ...search,
          user_ids:
            target.user_id !== undefined ? [target.user_id] : filters.user_ids,
          plan_ids:
            target.plan_id !== undefined ? [target.plan_id] : filters.plan_ids,
        },
      }
    }
    case 'invitation_paid_inviter':
      return {
        to: '/admin-analytics',
        search: {
          ...withAdminAnalyticsTab(
            withoutSubscriptionID(filters),
            'invitation-paid-subscriptions'
          ),
          inviter_id: target.inviter_id,
        },
      }
    case 'invitation_paid_invitee':
      return {
        to: '/admin-analytics',
        search: {
          ...withAdminAnalyticsTab(
            withoutSubscriptionID(filters),
            'invitation-paid-subscriptions'
          ),
          inviter_id: target.inviter_id,
          invitee_id: target.invitee_id,
        },
      }
    default:
      return null
  }
}
