import { formatTimestampToDate } from '@/lib/format'
import type {
  InvitationPaidInvitee,
  InvitationPaidInviter,
  InvitationPaidSubscriptionRecord,
  PaidSubscriptionValuePlanGroup,
  PaidSubscriptionValueSourceGroup,
  PaidSubscriptionValueSubscription,
  PaidSubscriptionValueUser,
} from '../types'
import {
  formatAdminMoneyAmount,
  formatAdminMoneyBreakdown,
  formatAdminPercent,
  formatAdminTokens,
} from './format'

export type AdminAnalyticsCardValue = { labelKey: string; value: string }

export function formatAdminAnalyticsTimestamp(
  value: number | null | undefined
): string {
  if (value === null || value === undefined || value <= 0) return '—'
  return formatTimestampToDate(value)
}

export function formatAdminAnalyticsOptionalValue(
  value: string | number | null | undefined
): string {
  if (value === null || value === undefined || value === '') return '—'
  return String(value)
}

export function paidSubscriptionValueUserCardValues(
  item: PaidSubscriptionValueUser
): AdminAnalyticsCardValue[] {
  return [
    {
      labelKey: 'adminAnalytics.fields.userId',
      value: formatAdminAnalyticsOptionalValue(item.user_id),
    },
    {
      labelKey: 'adminAnalytics.fields.displayName',
      value: formatAdminAnalyticsOptionalValue(item.display_name),
    },
    {
      labelKey: 'adminAnalytics.metrics.remainingValue',
      value: formatAdminMoneyBreakdown(
        item.recognized_remaining_value_by_currency
      ),
    },
    {
      labelKey: 'adminAnalytics.metrics.tokenBasedValue',
      value: formatAdminMoneyBreakdown(item.token_based_value_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.timeBasedValue',
      value: formatAdminMoneyBreakdown(item.time_based_value_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.activePaidSubscriptions',
      value: formatAdminTokens(item.active_paid_plan_count),
    },
    {
      labelKey: 'adminAnalytics.fields.earliestEndTime',
      value: formatAdminAnalyticsTimestamp(item.earliest_end_time),
    },
    {
      labelKey: 'adminAnalytics.fields.excludedReason',
      value: item.excluded ? item.excluded_reason || '—' : '—',
    },
    {
      labelKey: 'adminAnalytics.fields.excludedAt',
      value: formatAdminAnalyticsTimestamp(item.excluded_at),
    },
    {
      labelKey: 'adminAnalytics.fields.excludedBy',
      value: formatAdminAnalyticsOptionalValue(item.excluded_by),
    },
    {
      labelKey: 'adminAnalytics.fields.wouldHaveRemainingValue',
      value: formatAdminMoneyBreakdown(
        item.would_have_remaining_value_by_currency
      ),
    },
  ]
}

export function paidSubscriptionValuePlanCardValues(
  item: PaidSubscriptionValuePlanGroup
): AdminAnalyticsCardValue[] {
  return [
    {
      labelKey: 'adminAnalytics.metrics.remainingValue',
      value: formatAdminMoneyBreakdown(
        item.recognized_remaining_value_by_currency
      ),
    },
    {
      labelKey: 'adminAnalytics.metrics.tokenBasedValue',
      value: formatAdminMoneyBreakdown(item.token_based_value_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.timeBasedValue',
      value: formatAdminMoneyBreakdown(item.time_based_value_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.excludedAuditValue',
      value: formatAdminMoneyBreakdown(item.excluded_remaining_value_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.activePaidUsers',
      value: formatAdminTokens(item.active_user_count),
    },
    {
      labelKey: 'adminAnalytics.metrics.activePaidSubscriptions',
      value: formatAdminTokens(item.active_subscription_count),
    },
    {
      labelKey: 'adminAnalytics.fields.averageTokenUsageRatio',
      value: formatAdminPercent(item.average_token_usage_ratio),
    },
  ]
}

export function paidSubscriptionValueSourceCardValues(
  item: PaidSubscriptionValueSourceGroup
): AdminAnalyticsCardValue[] {
  return [
    {
      labelKey: 'adminAnalytics.fields.sourceAttribution',
      value: formatAdminAnalyticsOptionalValue(item.source_attribution),
    },
    {
      labelKey: 'adminAnalytics.metrics.remainingValue',
      value: formatAdminMoneyBreakdown(
        item.recognized_remaining_value_by_currency
      ),
    },
    {
      labelKey: 'adminAnalytics.metrics.excludedAuditValue',
      value: formatAdminMoneyBreakdown(item.excluded_remaining_value_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.activePaidUsers',
      value: formatAdminTokens(item.user_count),
    },
    {
      labelKey: 'adminAnalytics.metrics.subscriptionCount',
      value: formatAdminTokens(item.subscription_count),
    },
  ]
}

export function paidSubscriptionValueSubscriptionCardValues(
  item: PaidSubscriptionValueSubscription
): AdminAnalyticsCardValue[] {
  return [
    {
      labelKey: 'adminAnalytics.fields.subscriptionId',
      value: formatAdminAnalyticsOptionalValue(item.subscription_id),
    },
    {
      labelKey: 'adminAnalytics.fields.userId',
      value: formatAdminAnalyticsOptionalValue(item.user_id),
    },
    {
      labelKey: 'adminAnalytics.fields.planId',
      value: formatAdminAnalyticsOptionalValue(item.plan_id),
    },
    {
      labelKey: 'adminAnalytics.fields.planPrice',
      value: formatAdminMoneyAmount(item.plan_price),
    },
    {
      labelKey: 'adminAnalytics.fields.recognizedRemainingValue',
      value: formatAdminMoneyAmount(item.recognized_remaining_value),
    },
    {
      labelKey: 'adminAnalytics.metrics.tokenBasedValue',
      value: formatAdminMoneyAmount(item.token_based_value),
    },
    {
      labelKey: 'adminAnalytics.metrics.timeBasedValue',
      value: formatAdminMoneyAmount(item.time_based_value),
    },
    {
      labelKey: 'adminAnalytics.fields.startTime',
      value: formatAdminAnalyticsTimestamp(item.start_time),
    },
    {
      labelKey: 'adminAnalytics.fields.endTime',
      value: formatAdminAnalyticsTimestamp(item.end_time),
    },
    {
      labelKey: 'adminAnalytics.fields.remainingSeconds',
      value: formatAdminTokens(item.remaining_seconds),
    },
    {
      labelKey: 'adminAnalytics.fields.tokenLimit',
      value: formatAdminTokens(item.token_limit),
    },
    {
      labelKey: 'adminAnalytics.fields.tokenUsed',
      value: formatAdminTokens(item.token_used),
    },
    {
      labelKey: 'adminAnalytics.fields.nextResetTime',
      value: formatAdminAnalyticsTimestamp(item.next_reset_time),
    },
    {
      labelKey: 'adminAnalytics.fields.valuationBasis',
      value: formatAdminAnalyticsOptionalValue(item.valuation_basis),
    },
    {
      labelKey: 'adminAnalytics.fields.sourceAttribution',
      value: formatAdminAnalyticsOptionalValue(item.source_attribution),
    },
    {
      labelKey: 'adminAnalytics.fields.possibleOrderId',
      value: formatAdminAnalyticsOptionalValue(item.possible_order_id),
    },
    {
      labelKey: 'adminAnalytics.fields.paymentProvider',
      value: formatAdminAnalyticsOptionalValue(item.payment_provider),
    },
    {
      labelKey: 'adminAnalytics.fields.paymentMethod',
      value: formatAdminAnalyticsOptionalValue(item.payment_method),
    },
    {
      labelKey: 'adminAnalytics.fields.orderRecordedAmount',
      value: formatAdminMoneyAmount(item.order_recorded_amount),
    },
    {
      labelKey: 'adminAnalytics.fields.excludedReason',
      value: item.excluded ? item.excluded_reason || '—' : '—',
    },
  ]
}

export function invitationPaidInviterCardValues(
  item: InvitationPaidInviter
): AdminAnalyticsCardValue[] {
  return [
    {
      labelKey: 'adminAnalytics.fields.inviterUserId',
      value: formatAdminAnalyticsOptionalValue(item.inviter_user_id),
    },
    {
      labelKey: 'adminAnalytics.metrics.invitationPaidAmount',
      value: formatAdminMoneyBreakdown(
        item.recognized_invitation_paid_amount_by_currency
      ),
    },
    {
      labelKey: 'adminAnalytics.metrics.activeInvitationPaidAmount',
      value: formatAdminMoneyBreakdown(
        item.active_invitation_paid_amount_by_currency
      ),
    },
    {
      labelKey: 'adminAnalytics.metrics.activeInvitationRemainingValue',
      value: formatAdminMoneyBreakdown(
        item.active_invitation_remaining_value_by_currency
      ),
    },
    {
      labelKey: 'adminAnalytics.metrics.excludedInvitationPaidAmount',
      value: formatAdminMoneyBreakdown(
        item.excluded_invitation_paid_amount_by_currency
      ),
    },
    {
      labelKey: 'adminAnalytics.metrics.excludedActiveRemainingValue',
      value: formatAdminMoneyBreakdown(
        item.excluded_active_remaining_value_by_currency
      ),
    },
    {
      labelKey: 'adminAnalytics.metrics.inviteeCount',
      value: formatAdminTokens(item.invitee_count),
    },
    {
      labelKey: 'adminAnalytics.metrics.paidInviteeCount',
      value: formatAdminTokens(item.paid_invitee_count),
    },
    {
      labelKey: 'adminAnalytics.metrics.activePaidInviteeCount',
      value: formatAdminTokens(item.active_paid_invitee_count),
    },
    {
      labelKey: 'adminAnalytics.fields.latestPaidSubscriptionTime',
      value: formatAdminAnalyticsTimestamp(item.latest_paid_subscription_time),
    },
  ]
}

export function invitationPaidInviteeCardValues(
  item: InvitationPaidInvitee
): AdminAnalyticsCardValue[] {
  return [
    {
      labelKey: 'adminAnalytics.fields.inviteeUserId',
      value: formatAdminAnalyticsOptionalValue(item.invitee_user_id),
    },
    {
      labelKey: 'adminAnalytics.fields.inviterUserId',
      value: formatAdminAnalyticsOptionalValue(item.inviter_user_id),
    },
    {
      labelKey: 'adminAnalytics.fields.registeredAt',
      value: formatAdminAnalyticsTimestamp(item.registered_at),
    },
    {
      labelKey: 'adminAnalytics.fields.paidSubscriptionSnapshotCount',
      value: formatAdminTokens(item.paid_subscription_snapshot_count),
    },
    {
      labelKey: 'adminAnalytics.fields.confirmationUnits',
      value: formatAdminAnalyticsOptionalValue(item.recognized_paid_units),
    },
    {
      labelKey: 'adminAnalytics.metrics.invitationPaidAmount',
      value: formatAdminMoneyBreakdown(item.recognized_paid_amount_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.activeInvitationRemainingValue',
      value: formatAdminMoneyBreakdown(item.active_remaining_value_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.activeInvitationPaidAmount',
      value: formatAdminMoneyBreakdown(item.active_paid_amount_by_currency),
    },
    {
      labelKey: 'adminAnalytics.metrics.activePaidSubscriptions',
      value: formatAdminTokens(item.active_paid_subscription_count),
    },
    {
      labelKey: 'adminAnalytics.fields.excludedReason',
      value: item.excluded ? item.excluded_reason || '—' : '—',
    },
    {
      labelKey: 'adminAnalytics.fields.excludedAt',
      value: formatAdminAnalyticsTimestamp(item.excluded_at),
    },
    {
      labelKey: 'adminAnalytics.fields.excludedBy',
      value: formatAdminAnalyticsOptionalValue(item.excluded_by),
    },
    {
      labelKey: 'adminAnalytics.fields.wouldHavePaidAmount',
      value: formatAdminMoneyBreakdown(item.would_have_paid_amount_by_currency),
    },
    {
      labelKey: 'adminAnalytics.fields.wouldHaveActiveRemainingValue',
      value: formatAdminMoneyBreakdown(
        item.would_have_active_remaining_value_by_currency
      ),
    },
  ]
}

export function invitationPaidSubscriptionCardValues(
  item: InvitationPaidSubscriptionRecord
): AdminAnalyticsCardValue[] {
  return [
    {
      labelKey: 'adminAnalytics.fields.subscriptionId',
      value: formatAdminAnalyticsOptionalValue(item.subscription_id),
    },
    {
      labelKey: 'adminAnalytics.fields.inviteeUserId',
      value: formatAdminAnalyticsOptionalValue(item.invitee_user_id),
    },
    {
      labelKey: 'adminAnalytics.fields.inviterUserId',
      value: formatAdminAnalyticsOptionalValue(item.inviter_user_id),
    },
    {
      labelKey: 'adminAnalytics.fields.planId',
      value: formatAdminAnalyticsOptionalValue(item.plan_id),
    },
    {
      labelKey: 'adminAnalytics.fields.status',
      value: formatAdminAnalyticsOptionalValue(item.status),
    },
    {
      labelKey: 'adminAnalytics.fields.planPrice',
      value: formatAdminMoneyAmount(item.plan_price),
    },
    {
      labelKey: 'adminAnalytics.fields.confirmationUnits',
      value: formatAdminAnalyticsOptionalValue(item.recognized_paid_units),
    },
    {
      labelKey: 'adminAnalytics.fields.invitationPaidAmount',
      value: formatAdminMoneyAmount(item.recognized_paid_amount),
    },
    {
      labelKey: 'adminAnalytics.fields.recognizedRemainingValue',
      value: formatAdminMoneyAmount(item.recognized_remaining_value),
    },
    {
      labelKey: 'adminAnalytics.fields.startTime',
      value: formatAdminAnalyticsTimestamp(item.start_time),
    },
    {
      labelKey: 'adminAnalytics.fields.endTime',
      value: formatAdminAnalyticsTimestamp(item.end_time),
    },
    {
      labelKey: 'adminAnalytics.fields.unitInferenceBasis',
      value: formatAdminAnalyticsOptionalValue(item.unit_inference_basis),
    },
    {
      labelKey: 'adminAnalytics.fields.sourceAttribution',
      value: formatAdminAnalyticsOptionalValue(item.source_attribution),
    },
    {
      labelKey: 'adminAnalytics.fields.excludedReason',
      value: item.excluded ? item.excluded_reason || '—' : '—',
    },
    {
      labelKey: 'adminAnalytics.fields.possibleOrderId',
      value: formatAdminAnalyticsOptionalValue(item.possible_order_id),
    },
    {
      labelKey: 'adminAnalytics.fields.paymentProvider',
      value: formatAdminAnalyticsOptionalValue(item.payment_provider),
    },
    {
      labelKey: 'adminAnalytics.fields.paymentMethod',
      value: formatAdminAnalyticsOptionalValue(item.payment_method),
    },
    {
      labelKey: 'adminAnalytics.fields.orderRecordedAmount',
      value: formatAdminMoneyAmount(item.order_recorded_amount),
    },
    {
      labelKey: 'adminAnalytics.fields.orderStatus',
      value: formatAdminAnalyticsOptionalValue(item.order_status),
    },
    {
      labelKey: 'adminAnalytics.fields.completeTime',
      value: formatAdminAnalyticsTimestamp(item.complete_time),
    },
  ]
}
