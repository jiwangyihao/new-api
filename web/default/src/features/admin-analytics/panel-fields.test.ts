import assert from 'node:assert/strict'
import test from 'node:test'
import {
  invitationPaidInviteeCardValues,
  invitationPaidInviterCardValues,
  invitationPaidSubscriptionCardValues,
  paidSubscriptionValuePlanCardValues,
  paidSubscriptionValueSourceCardValues,
  paidSubscriptionValueSubscriptionCardValues,
  paidSubscriptionValueUserCardValues,
} from './lib/panel-fields'
import type {
  InvitationPaidInvitee,
  InvitationPaidInviter,
  InvitationPaidSubscriptionRecord,
  MoneyAmount,
  PaidSubscriptionValuePlanGroup,
  PaidSubscriptionValueSourceGroup,
  PaidSubscriptionValueSubscription,
  PaidSubscriptionValueUser,
} from './types'

function labelKeys(values: Array<{ labelKey: string }>): string[] {
  return values.map((value) => value.labelKey)
}

function cny(amount: number): MoneyAmount {
  return { amount, currency: 'CNY' }
}

test('paid subscription value user cards expose returned detail fields', () => {
  const item: PaidSubscriptionValueUser = {
    user_id: 1,
    username: 'alice',
    display_name: 'Alice',
    active_paid_plan_count: 2,
    recognized_remaining_value_by_currency: [cny(10)],
    token_based_value_by_currency: [cny(9)],
    time_based_value_by_currency: [cny(8)],
    earliest_end_time: 1_700_000_000,
    excluded: true,
    excluded_reason: 'ops',
    excluded_at: 1_700_000_001,
    excluded_by: 7,
    would_have_remaining_value_by_currency: [cny(11)],
  }

  assert.deepEqual(labelKeys(paidSubscriptionValueUserCardValues(item)), [
    'adminAnalytics.fields.userId',
    'adminAnalytics.fields.displayName',
    'adminAnalytics.metrics.remainingValue',
    'adminAnalytics.metrics.tokenBasedValue',
    'adminAnalytics.metrics.timeBasedValue',
    'adminAnalytics.metrics.activePaidSubscriptions',
    'adminAnalytics.fields.earliestEndTime',
    'adminAnalytics.fields.excludedReason',
    'adminAnalytics.fields.excludedAt',
    'adminAnalytics.fields.excludedBy',
    'adminAnalytics.fields.wouldHaveRemainingValue',
  ])
})

test('paid subscription value group cards expose value source and audit fields', () => {
  const plan: PaidSubscriptionValuePlanGroup = {
    plan_id: 2,
    plan_name: 'Pro',
    plan_business_code: 'pro',
    active_user_count: 3,
    active_subscription_count: 4,
    recognized_remaining_value_by_currency: [cny(10)],
    token_based_value_by_currency: [cny(9)],
    time_based_value_by_currency: [cny(8)],
    excluded_remaining_value_by_currency: [cny(7)],
    average_token_usage_ratio: 0.5,
  }
  const source: PaidSubscriptionValueSourceGroup = {
    source: 'order',
    grant_reason: 'order',
    user_count: 3,
    subscription_count: 4,
    recognized_remaining_value_by_currency: [cny(10)],
    excluded_remaining_value_by_currency: [cny(7)],
    source_attribution: 'order',
  }

  assert.deepEqual(labelKeys(paidSubscriptionValuePlanCardValues(plan)), [
    'adminAnalytics.metrics.remainingValue',
    'adminAnalytics.metrics.tokenBasedValue',
    'adminAnalytics.metrics.timeBasedValue',
    'adminAnalytics.metrics.excludedAuditValue',
    'adminAnalytics.metrics.activePaidUsers',
    'adminAnalytics.metrics.activePaidSubscriptions',
    'adminAnalytics.fields.averageTokenUsageRatio',
  ])
  assert.deepEqual(labelKeys(paidSubscriptionValueSourceCardValues(source)), [
    'adminAnalytics.fields.sourceAttribution',
    'adminAnalytics.metrics.remainingValue',
    'adminAnalytics.metrics.excludedAuditValue',
    'adminAnalytics.metrics.activePaidUsers',
    'adminAnalytics.metrics.subscriptionCount',
  ])
})

test('paid subscription value records expose valuation and order traceability fields', () => {
  const item: PaidSubscriptionValueSubscription = {
    subscription_id: 3,
    user_id: 1,
    username: 'alice',
    plan_id: 2,
    plan_name: 'Pro',
    source: 'order',
    grant_reason: 'order',
    plan_price: cny(30),
    start_time: 1,
    end_time: 2,
    remaining_seconds: 3,
    token_limit: 4,
    token_used: 5,
    next_reset_time: 6,
    token_based_value: cny(7),
    time_based_value: cny(8),
    recognized_remaining_value: cny(7),
    valuation_basis: 'minimum_of_token_time',
    source_attribution: 'order',
    excluded: false,
    excluded_reason: '',
    possible_order_id: 9,
    payment_provider: 'kyren',
    payment_method: 'alipay',
    order_recorded_amount: cny(30),
  }

  assert.deepEqual(
    labelKeys(paidSubscriptionValueSubscriptionCardValues(item)),
    [
      'adminAnalytics.fields.subscriptionId',
      'adminAnalytics.fields.userId',
      'adminAnalytics.fields.planId',
      'adminAnalytics.fields.planPrice',
      'adminAnalytics.fields.recognizedRemainingValue',
      'adminAnalytics.metrics.tokenBasedValue',
      'adminAnalytics.metrics.timeBasedValue',
      'adminAnalytics.fields.startTime',
      'adminAnalytics.fields.endTime',
      'adminAnalytics.fields.remainingSeconds',
      'adminAnalytics.fields.tokenLimit',
      'adminAnalytics.fields.tokenUsed',
      'adminAnalytics.fields.nextResetTime',
      'adminAnalytics.fields.valuationBasis',
      'adminAnalytics.fields.sourceAttribution',
      'adminAnalytics.fields.possibleOrderId',
      'adminAnalytics.fields.paymentProvider',
      'adminAnalytics.fields.paymentMethod',
      'adminAnalytics.fields.orderRecordedAmount',
      'adminAnalytics.fields.excludedReason',
    ]
  )
})

test('invitation paid inviter cards expose amount count and latest paid fields', () => {
  const item: InvitationPaidInviter = {
    inviter_user_id: 1,
    inviter_username: 'alice',
    invitee_count: 2,
    paid_invitee_count: 1,
    active_paid_invitee_count: 1,
    recognized_invitation_paid_amount_by_currency: [cny(30)],
    active_invitation_paid_amount_by_currency: [cny(30)],
    active_invitation_remaining_value_by_currency: [cny(10)],
    excluded_invitation_paid_amount_by_currency: [cny(5)],
    excluded_active_remaining_value_by_currency: [cny(2)],
    latest_paid_subscription_time: 1_700_000_000,
  }

  assert.deepEqual(labelKeys(invitationPaidInviterCardValues(item)), [
    'adminAnalytics.fields.inviterUserId',
    'adminAnalytics.metrics.invitationPaidAmount',
    'adminAnalytics.metrics.activeInvitationPaidAmount',
    'adminAnalytics.metrics.activeInvitationRemainingValue',
    'adminAnalytics.metrics.excludedInvitationPaidAmount',
    'adminAnalytics.metrics.excludedActiveRemainingValue',
    'adminAnalytics.metrics.inviteeCount',
    'adminAnalytics.metrics.paidInviteeCount',
    'adminAnalytics.metrics.activePaidInviteeCount',
    'adminAnalytics.fields.latestPaidSubscriptionTime',
  ])
})

test('invitation paid invitee cards expose ids audit and would-have fields', () => {
  const item: InvitationPaidInvitee = {
    invitee_user_id: 2,
    invitee_username: 'bob',
    inviter_user_id: 1,
    registered_at: 1,
    paid_subscription_snapshot_count: 2,
    recognized_paid_units: 1,
    active_paid_subscription_count: 1,
    recognized_paid_amount_by_currency: [cny(30)],
    active_remaining_value_by_currency: [cny(10)],
    active_paid_amount_by_currency: [cny(30)],
    excluded: true,
    excluded_reason: 'ops',
    excluded_at: 7,
    excluded_by: 8,
    would_have_paid_amount_by_currency: [cny(30)],
    would_have_active_remaining_value_by_currency: [cny(10)],
  }

  assert.deepEqual(labelKeys(invitationPaidInviteeCardValues(item)), [
    'adminAnalytics.fields.inviteeUserId',
    'adminAnalytics.fields.inviterUserId',
    'adminAnalytics.fields.registeredAt',
    'adminAnalytics.fields.paidSubscriptionSnapshotCount',
    'adminAnalytics.fields.confirmationUnits',
    'adminAnalytics.metrics.invitationPaidAmount',
    'adminAnalytics.metrics.activeInvitationRemainingValue',
    'adminAnalytics.metrics.activeInvitationPaidAmount',
    'adminAnalytics.metrics.activePaidSubscriptions',
    'adminAnalytics.fields.excludedReason',
    'adminAnalytics.fields.excludedAt',
    'adminAnalytics.fields.excludedBy',
    'adminAnalytics.fields.wouldHavePaidAmount',
    'adminAnalytics.fields.wouldHaveActiveRemainingValue',
  ])
})

test('invitation paid records expose entitlement order and completion fields', () => {
  const item: InvitationPaidSubscriptionRecord = {
    subscription_id: 3,
    invitee_user_id: 2,
    inviter_user_id: 1,
    plan_id: 4,
    plan_name: 'Pro',
    plan_price: cny(30),
    recognized_paid_units: 1,
    recognized_paid_amount: cny(30),
    unit_inference_basis: 'period_aligned',
    source: 'order',
    grant_reason: 'order',
    source_attribution: 'order',
    start_time: 1,
    end_time: 2,
    status: 'active',
    recognized_remaining_value: cny(10),
    excluded: false,
    excluded_reason: '',
    possible_order_id: 9,
    payment_provider: 'kyren',
    payment_method: 'alipay',
    order_recorded_amount: cny(30),
    order_status: 'paid',
    complete_time: 10,
  }

  assert.deepEqual(labelKeys(invitationPaidSubscriptionCardValues(item)), [
    'adminAnalytics.fields.subscriptionId',
    'adminAnalytics.fields.inviteeUserId',
    'adminAnalytics.fields.inviterUserId',
    'adminAnalytics.fields.planId',
    'adminAnalytics.fields.status',
    'adminAnalytics.fields.planPrice',
    'adminAnalytics.fields.confirmationUnits',
    'adminAnalytics.fields.invitationPaidAmount',
    'adminAnalytics.fields.recognizedRemainingValue',
    'adminAnalytics.fields.startTime',
    'adminAnalytics.fields.endTime',
    'adminAnalytics.fields.unitInferenceBasis',
    'adminAnalytics.fields.sourceAttribution',
    'adminAnalytics.fields.excludedReason',
    'adminAnalytics.fields.possibleOrderId',
    'adminAnalytics.fields.paymentProvider',
    'adminAnalytics.fields.paymentMethod',
    'adminAnalytics.fields.orderRecordedAmount',
    'adminAnalytics.fields.orderStatus',
    'adminAnalytics.fields.completeTime',
  ])
})
