/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import test from 'node:test'
import {
  adminAnalyticsCreditOverviewValues,
  adminAnalyticsCreditRankingValue,
  adminAnalyticsSubscriptionHistoryValues,
  invitationPaidInviteeCardValues,
  invitationPaidInviterCardValues,
  invitationPaidSubscriptionCardValues,
  paidSubscriptionValuePlanCardValues,
  paidSubscriptionValueSourceCardValues,
  paidSubscriptionValueSubscriptionCardValues,
  paidSubscriptionValueUserCardValues,
} from './lib/panel-fields'
import type {
  AdminAnalyticsDrilldownSubscriptionItem,
  AdminAnalyticsOverviewQuota,
  AdminAnalyticsOverviewSubscriptions,
  AdminAnalyticsSubscriptionRankingItem,
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
      'adminAnalytics.fields.valuationConfidence',
      'adminAnalytics.fields.valuationWarnings',
      'adminAnalytics.fields.sourceAttribution',
      'adminAnalytics.fields.possibleOrderId',
      'adminAnalytics.fields.paymentProvider',
      'adminAnalytics.fields.paymentMethod',
      'adminAnalytics.fields.orderRecordedAmount',
      'adminAnalytics.fields.excludedReason',
    ]
  )
})

test('timed subscription value records render per-currency values when singular amounts are null', () => {
  const item = {
    subscription_id: 4,
    user_id: 2,
    username: 'multi-currency-user',
    plan_id: 3,
    plan_name: 'Timed Pro',
    entitlement_type: 'timed',
    source: 'admin',
    grant_reason: 'after_sales',
    plan_price: cny(40),
    start_time: 1,
    end_time: 2,
    remaining_seconds: 3,
    token_limit: 100,
    token_used: 50,
    next_reset_time: 6,
    token_based_value: null,
    time_based_value: null,
    recognized_remaining_value: null,
    token_based_value_by_currency: [cny(12), { amount: 6, currency: 'USD' }],
    time_based_value_by_currency: [cny(20), { amount: 10, currency: 'USD' }],
    recognized_remaining_value_by_currency: [
      cny(10),
      { amount: 5, currency: 'USD' },
    ],
    valuation_basis: 'minimum_of_token_time',
    valuation_confidence: 'exact',
    valuation_warnings: ['overlapping_grants'],
    source_attribution: 'mixed_grants',
    excluded: false,
    excluded_reason: '',
  } as unknown as PaidSubscriptionValueSubscription

  const values = Object.fromEntries(
    paidSubscriptionValueSubscriptionCardValues(item).map((value) => [
      value.labelKey,
      value.value ?? value.valueKey,
    ])
  )

  assert.equal(
    values['adminAnalytics.fields.recognizedRemainingValue'],
    '¥10.00, $5.00'
  )
  assert.equal(
    values['adminAnalytics.metrics.tokenBasedValue'],
    '¥12.00, $6.00'
  )
  assert.equal(
    values['adminAnalytics.metrics.timeBasedValue'],
    '¥20.00, $10.00'
  )
  assert.equal(
    values['adminAnalytics.fields.valuationConfidence'],
    'adminAnalytics.valuationConfidence.exact'
  )
  assert.equal(
    values['adminAnalytics.fields.valuationWarnings'],
    'overlapping_grants'
  )
})

test('Credit value records expose exact valuation and localized semantics', () => {
  const item: PaidSubscriptionValueSubscription = {
    subscription_id: 4,
    user_id: 1,
    username: 'alice',
    plan_id: 3,
    plan_name: 'Credit balance',
    source: 'credit_balance_pool',
    grant_reason: 'order',
    plan_price: { amount: 0, amount_micros: '0', currency: 'CNY' },
    start_time: 1,
    end_time: 0,
    remaining_seconds: 0,
    token_limit: 1_000,
    token_used: 200,
    available_credit: 800,
    unknown_cost_credit: 0,
    next_reset_time: 0,
    token_based_value: {
      amount: 32,
      amount_micros: '32000000',
      currency: 'CNY',
    },
    time_based_value: null,
    recognized_remaining_value: {
      amount: 32,
      amount_micros: '32000000',
      currency: 'CNY',
    },
    exact_remaining_value: {
      amount: 32,
      amount_micros: '32000000',
      currency: 'CNY',
    },
    estimated_remaining_value: {
      amount: 0,
      amount_micros: '0',
      currency: 'CNY',
    },
    valuation_basis: 'credit_moving_weighted_average',
    valuation_confidence: 'exact',
    valuation_state_version: 2,
    valuation_updated_at: 1_700_000_000,
    snapshot_semantics: 'current_only',
    entitlement_type: 'credit_balance',
    source_attribution: 'moving_weighted_pool',
    excluded: false,
    excluded_reason: '',
  }

  const values = paidSubscriptionValueSubscriptionCardValues(item)
  const byLabel = new Map(values.map((value) => [value.labelKey, value]))
  assert.equal(
    byLabel.get('adminAnalytics.fields.recognizedRemainingValue')?.value,
    '¥32.00'
  )
  assert.equal(
    byLabel.get('adminAnalytics.fields.exactRemainingValue')?.value,
    '¥32.00'
  )
  assert.equal(
    byLabel.get('adminAnalytics.metrics.timeBasedValue')?.valueKey,
    'adminAnalytics.values.notApplicable'
  )
  assert.equal(
    byLabel.get('adminAnalytics.fields.valuationBasis')?.valueKey,
    'adminAnalytics.valuationBasis.creditMovingWeightedAverage'
  )
  assert.equal(
    byLabel.get('adminAnalytics.fields.valuationConfidence')?.valueKey,
    'adminAnalytics.valuationConfidence.exact'
  )
  assert.equal(
    byLabel.get('adminAnalytics.fields.snapshotSemantics')?.valueKey,
    'adminAnalytics.snapshotSemantics.currentOnly'
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

test('Credit overview values keep availability debt and lifecycle counts separate', () => {
  const quota: AdminAnalyticsOverviewQuota = {
    token_limit: 1_000,
    token_used: 700,
    remaining_tokens: 300,
    usage_rate: 0.7,
    available_credit: 125,
    settlement_debt: 30,
  }
  const subscriptions: AdminAnalyticsOverviewSubscriptions = {
    active_count: 4,
    expired_count: 2,
    trial_count: 1,
    paid_count: 3,
    reward_count: 0,
    timed_active_count: 2,
    credit_balance_count: 3,
    credit_available_count: 1,
    credit_exhausted_count: 1,
    credit_debt_count: 1,
  }

  const values = adminAnalyticsCreditOverviewValues(quota, subscriptions)

  assert.deepEqual(labelKeys(values), [
    'adminAnalytics.metrics.availableCredit',
    'adminAnalytics.metrics.settlementDebt',
    'adminAnalytics.metrics.timedActiveSubscriptions',
    'adminAnalytics.metrics.creditBalanceCount',
    'adminAnalytics.metrics.creditAvailableCount',
    'adminAnalytics.metrics.creditExhaustedCount',
    'adminAnalytics.metrics.creditDebtCount',
  ])
  assert.deepEqual(
    values.map((value) => value.value),
    ['125', '30', '2', '3', '1', '1', '1']
  )
})

test('quota ranking value exposes the entitlement lifecycle instead of treating Credit as timed quota', () => {
  const credit: AdminAnalyticsSubscriptionRankingItem = {
    subscription_id: 31,
    user_id: 7,
    username: 'credit-user',
    plan_id: 9,
    plan_title: 'Credit',
    source: 'order',
    status: 'active',
    start_time: 1,
    end_time: 0,
    token_limit: 100,
    token_used: 130,
    remaining_tokens: 0,
    usage_rate: 1.3,
    request_count: 4,
    entitlement_type: 'credit_balance',
    lifecycle_state: 'credit_debt',
    available_credit: 0,
    settlement_debt: 30,
  }

  assert.deepEqual(adminAnalyticsCreditRankingValue(credit), {
    labelKey: 'adminAnalytics.lifecycle.creditDebt',
    value: '30',
  })
})

test('subscription history cards expose source lifecycle conversion and validated target fields', () => {
  const item: AdminAnalyticsDrilldownSubscriptionItem = {
    subscription_id: 41,
    user_id: 8,
    username: 'converted-user',
    plan_id: 10,
    plan_title: 'Timed Pro',
    source: 'order',
    status: 'converted',
    start_time: 1_700_000_000,
    end_time: 1_700_100_000,
    token_limit: 1_000,
    token_used: 200,
    remaining_tokens: 800,
    usage_rate: 0.2,
    entitlement_type: 'timed',
    lifecycle_state: 'converted',
    available_credit: 0,
    settlement_debt: 0,
    grace_remaining_seconds: 0,
    conversion_id: 51,
    target_subscription_id: 42,
    target_user_id: 8,
    target_plan_id: 11,
    target_plan_title: 'Credit Balance',
  }

  assert.deepEqual(labelKeys(adminAnalyticsSubscriptionHistoryValues(item)), [
    'adminAnalytics.fields.subscriptionId',
    'adminAnalytics.fields.userId',
    'adminAnalytics.fields.planId',
    'adminAnalytics.fields.entitlementType',
    'adminAnalytics.fields.lifecycleState',
    'adminAnalytics.fields.status',
    'adminAnalytics.fields.sourceAttribution',
    'adminAnalytics.fields.availableCredit',
    'adminAnalytics.fields.settlementDebt',
    'adminAnalytics.fields.graceRemainingSeconds',
    'adminAnalytics.fields.conversionId',
    'adminAnalytics.fields.targetSubscriptionId',
    'adminAnalytics.fields.targetUserId',
    'adminAnalytics.fields.targetPlanId',
    'adminAnalytics.fields.targetPlanTitle',
    'adminAnalytics.fields.startTime',
    'adminAnalytics.fields.endTime',
  ])
})
