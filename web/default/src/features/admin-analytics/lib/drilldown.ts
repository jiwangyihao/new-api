import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsDrilldownTarget,
  FrontendAdminAnalyticsDrilldownTarget,
} from '../types'

function secondsToMillis(value: number | undefined): number | undefined {
  return value === undefined ? undefined : value * 1000
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
          ...filters,
          tab: target.kind === 'admin_subscriptions' ? 'plans' : 'invitations',
          user_id: target.user_id,
          plan_id: target.plan_id,
          inviter_id: target.inviter_id,
        },
      }
    default:
      return null
  }
}
