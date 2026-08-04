import { formatTimestampToDate } from '@/lib/format'
import type {
  AdminAnalyticsDrilldownSubscriptionItem,
  AdminAnalyticsOverviewQuota,
  AdminAnalyticsOverviewSubscriptions,
  AdminAnalyticsSubscriptionLifecycleState,
  AdminAnalyticsSubscriptionRankingItem,
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

export type AdminAnalyticsCardValue = {
  labelKey: string
  value?: string
  valueKey?: string
}

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

export const adminAnalyticsLifecycleLabelKeys: Record<
  AdminAnalyticsSubscriptionLifecycleState,
  string
> = {
  active_timed: 'adminAnalytics.lifecycle.activeTimed',
  active_credit: 'adminAnalytics.lifecycle.activeCredit',
  exhausted_credit: 'adminAnalytics.lifecycle.exhaustedCredit',
  credit_debt: 'adminAnalytics.lifecycle.creditDebt',
  expired_grace: 'adminAnalytics.lifecycle.expiredGrace',
  converted: 'adminAnalytics.lifecycle.converted',
}

const adminAnalyticsValuationBasisLabelKeys: Record<string, string> = {
  credit_moving_weighted_average:
    'adminAnalytics.valuationBasis.creditMovingWeightedAverage',
}

const adminAnalyticsValuationConfidenceLabelKeys: Record<string, string> = {
  exact: 'adminAnalytics.valuationConfidence.exact',
  estimated: 'adminAnalytics.valuationConfidence.estimated',
  mixed: 'adminAnalytics.valuationConfidence.mixed',
  unknown: 'adminAnalytics.valuationConfidence.unknown',
}

const adminAnalyticsSnapshotSemanticsLabelKeys: Record<string, string> = {
  snapshot: 'adminAnalytics.snapshotSemantics.snapshot',
  current_only: 'adminAnalytics.snapshotSemantics.currentOnly',
}

const adminAnalyticsSourceAttributionLabelKeys: Record<string, string> = {
  moving_weighted_pool: 'adminAnalytics.sourceAttribution.movingWeightedPool',
}

function localizedAdminAnalyticsValue(
  value: string | null | undefined,
  labelKeys: Record<string, string>
): Pick<AdminAnalyticsCardValue, 'value' | 'valueKey'> {
  const valueKey = value ? labelKeys[value] : undefined
  return valueKey
    ? { valueKey }
    : { value: formatAdminAnalyticsOptionalValue(value) }
}

export function adminAnalyticsCreditOverviewValues(
  quota: AdminAnalyticsOverviewQuota,
  subscriptions: AdminAnalyticsOverviewSubscriptions
): Array<AdminAnalyticsCardValue & { value: string }> {
  return [
    {
      labelKey: 'adminAnalytics.metrics.availableCredit',
      value: formatAdminTokens(quota.available_credit),
    },
    {
      labelKey: 'adminAnalytics.metrics.settlementDebt',
      value: formatAdminTokens(quota.settlement_debt),
    },
    {
      labelKey: 'adminAnalytics.metrics.timedActiveSubscriptions',
      value: formatAdminTokens(subscriptions.timed_active_count),
    },
    {
      labelKey: 'adminAnalytics.metrics.creditBalanceCount',
      value: formatAdminTokens(subscriptions.credit_balance_count),
    },
    {
      labelKey: 'adminAnalytics.metrics.creditAvailableCount',
      value: formatAdminTokens(subscriptions.credit_available_count),
    },
    {
      labelKey: 'adminAnalytics.metrics.creditExhaustedCount',
      value: formatAdminTokens(subscriptions.credit_exhausted_count),
    },
    {
      labelKey: 'adminAnalytics.metrics.creditDebtCount',
      value: formatAdminTokens(subscriptions.credit_debt_count),
    },
  ]
}

export function adminAnalyticsCreditRankingValue(
  item: AdminAnalyticsSubscriptionRankingItem
): AdminAnalyticsCardValue & { value: string } {
  const labelKey = adminAnalyticsLifecycleLabelKeys[item.lifecycle_state]
  const amount =
    item.lifecycle_state === 'credit_debt'
      ? item.settlement_debt
      : item.entitlement_type === 'credit_balance'
        ? item.available_credit
        : item.remaining_tokens
  return { labelKey, value: formatAdminTokens(amount) }
}

export function adminAnalyticsSubscriptionHistoryValues(
  item: AdminAnalyticsDrilldownSubscriptionItem
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
      labelKey: 'adminAnalytics.fields.entitlementType',
      value: formatAdminAnalyticsOptionalValue(item.entitlement_type),
    },
    {
      labelKey: 'adminAnalytics.fields.lifecycleState',
      value: formatAdminAnalyticsOptionalValue(item.lifecycle_state),
    },
    {
      labelKey: 'adminAnalytics.fields.status',
      value: formatAdminAnalyticsOptionalValue(item.status),
    },
    {
      labelKey: 'adminAnalytics.fields.sourceAttribution',
      value: formatAdminAnalyticsOptionalValue(item.source),
    },
    {
      labelKey: 'adminAnalytics.fields.availableCredit',
      value: formatAdminTokens(item.available_credit),
    },
    {
      labelKey: 'adminAnalytics.fields.settlementDebt',
      value: formatAdminTokens(item.settlement_debt),
    },
    {
      labelKey: 'adminAnalytics.fields.graceRemainingSeconds',
      value: formatAdminTokens(item.grace_remaining_seconds),
    },
    {
      labelKey: 'adminAnalytics.fields.conversionId',
      value: formatAdminAnalyticsOptionalValue(item.conversion_id),
    },
    {
      labelKey: 'adminAnalytics.fields.targetSubscriptionId',
      value: formatAdminAnalyticsOptionalValue(item.target_subscription_id),
    },
    {
      labelKey: 'adminAnalytics.fields.targetUserId',
      value: formatAdminAnalyticsOptionalValue(item.target_user_id),
    },
    {
      labelKey: 'adminAnalytics.fields.targetPlanId',
      value: formatAdminAnalyticsOptionalValue(item.target_plan_id),
    },
    {
      labelKey: 'adminAnalytics.fields.targetPlanTitle',
      value: formatAdminAnalyticsOptionalValue(item.target_plan_title),
    },
    {
      labelKey: 'adminAnalytics.fields.startTime',
      value: formatAdminAnalyticsTimestamp(item.start_time),
    },
    {
      labelKey: 'adminAnalytics.fields.endTime',
      value: formatAdminAnalyticsTimestamp(item.end_time),
    },
  ]
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
      value: formatAdminMoneyBreakdown(
        item.excluded_remaining_value_by_currency
      ),
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
      value: formatAdminMoneyBreakdown(
        item.excluded_remaining_value_by_currency
      ),
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
      value:
        item.recognized_remaining_value === null
          ? formatAdminMoneyBreakdown(
              item.recognized_remaining_value_by_currency
            )
          : formatAdminMoneyAmount(item.recognized_remaining_value),
    },
    ...(item.exact_remaining_value === undefined
      ? []
      : [
          {
            labelKey: 'adminAnalytics.fields.exactRemainingValue',
            value: formatAdminMoneyAmount(item.exact_remaining_value),
          },
        ]),
    ...(item.estimated_remaining_value === undefined
      ? []
      : [
          {
            labelKey: 'adminAnalytics.fields.estimatedRemainingValue',
            value: formatAdminMoneyAmount(item.estimated_remaining_value),
          },
        ]),
    {
      labelKey: 'adminAnalytics.metrics.tokenBasedValue',
      value:
        item.token_based_value === null
          ? formatAdminMoneyBreakdown(item.token_based_value_by_currency)
          : formatAdminMoneyAmount(item.token_based_value),
    },
    item.time_based_value === null && item.entitlement_type === 'credit_balance'
      ? {
          labelKey: 'adminAnalytics.metrics.timeBasedValue',
          valueKey: 'adminAnalytics.values.notApplicable',
        }
      : {
          labelKey: 'adminAnalytics.metrics.timeBasedValue',
          value:
            item.time_based_value === null
              ? formatAdminMoneyBreakdown(item.time_based_value_by_currency)
              : formatAdminMoneyAmount(item.time_based_value),
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
    ...(item.available_credit === undefined
      ? []
      : [
          {
            labelKey: 'adminAnalytics.fields.availableCredit',
            value: formatAdminTokens(item.available_credit),
          },
        ]),
    ...(item.unknown_cost_credit === undefined
      ? []
      : [
          {
            labelKey: 'adminAnalytics.fields.unknownCostCredit',
            value: formatAdminTokens(item.unknown_cost_credit),
          },
        ]),
    {
      labelKey: 'adminAnalytics.fields.nextResetTime',
      value: formatAdminAnalyticsTimestamp(item.next_reset_time),
    },
    {
      labelKey: 'adminAnalytics.fields.valuationBasis',
      ...localizedAdminAnalyticsValue(
        item.valuation_basis,
        adminAnalyticsValuationBasisLabelKeys
      ),
    },
    {
      labelKey: 'adminAnalytics.fields.valuationConfidence',
      ...localizedAdminAnalyticsValue(
        item.valuation_confidence,
        adminAnalyticsValuationConfidenceLabelKeys
      ),
    },
    ...(item.valuation_state_version === undefined
      ? []
      : [
          {
            labelKey: 'adminAnalytics.fields.valuationStateVersion',
            value: formatAdminTokens(item.valuation_state_version),
          },
        ]),
    ...(item.valuation_updated_at === undefined
      ? []
      : [
          {
            labelKey: 'adminAnalytics.fields.valuationUpdatedAt',
            value: formatAdminAnalyticsTimestamp(item.valuation_updated_at),
          },
        ]),
    ...(item.snapshot_semantics === undefined
      ? []
      : [
          {
            labelKey: 'adminAnalytics.fields.snapshotSemantics',
            ...localizedAdminAnalyticsValue(
              item.snapshot_semantics,
              adminAnalyticsSnapshotSemanticsLabelKeys
            ),
          },
        ]),
    ...(item.entitlement_type === undefined
      ? []
      : [
          {
            labelKey: 'adminAnalytics.fields.entitlementType',
            value: formatAdminAnalyticsOptionalValue(item.entitlement_type),
          },
        ]),
    {
      labelKey: 'adminAnalytics.fields.valuationWarnings',
      value:
        item.valuation_warnings && item.valuation_warnings.length > 0
          ? item.valuation_warnings.join(', ')
          : '—',
    },
    {
      labelKey: 'adminAnalytics.fields.sourceAttribution',
      ...localizedAdminAnalyticsValue(
        item.source_attribution,
        adminAnalyticsSourceAttributionLabelKeys
      ),
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
