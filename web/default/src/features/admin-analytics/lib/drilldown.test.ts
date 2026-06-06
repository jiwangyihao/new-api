import assert from 'node:assert/strict'
import test from 'node:test'
import { buildAdminAnalyticsDrilldown } from './drilldown'
import { buildAdminAnalyticsCanonicalFilters } from './filters'

const filters = buildAdminAnalyticsCanonicalFilters({
  start_timestamp: 10,
  end_timestamp: 20,
})

test('unknown target kind is rejected', () => {
  assert.equal(buildAdminAnalyticsDrilldown(filters, { kind: 'bad' }), null)
})

test('usage logs target uses common section and milliseconds', () => {
  const target = buildAdminAnalyticsDrilldown(filters, {
    kind: 'admin_usage_logs',
    user_id: 1,
    token_id: 2,
    model: 'gpt',
  })
  assert.equal(target?.to, '/usage-logs/$section')
  assert.deepEqual(target?.params, { section: 'common' })
  assert.equal(target?.search.startTime, 10_000)
  assert.equal(target?.search.endTime, 20_000)
  assert.equal(target?.search.tokenId, 2)
})

test('users target does not create a route navigation', () => {
  const target = buildAdminAnalyticsDrilldown(filters, {
    kind: 'admin_users',
    user_id: 1,
    plan_id: 2,
    inviter_id: 3,
  })

  assert.equal(target, null)
})


test('admin analytics targets keep canonical repeated filters', () => {
  const target = buildAdminAnalyticsDrilldown(filters, {
    kind: 'admin_subscriptions',
    user_id: 3,
    plan_id: 4,
  })

  assert.equal(target?.to, '/admin-analytics')
  assert.equal(target?.search.tab, 'plans')
  assert.deepEqual(target?.search.user_ids, [3])
  assert.deepEqual(target?.search.plan_ids, [4])
  assert.equal(target?.search.user_id, undefined)
  assert.equal(target?.search.plan_id, undefined)
})

test('paid subscription value subscription target writes subscription id filter', () => {
  const target = buildAdminAnalyticsDrilldown(filters, {
    kind: 'paid_subscription_value_subscription',
    subscription_id: 11,
    user_id: 3,
    plan_id: 4,
  })

  assert.equal(target?.to, '/admin-analytics')
  assert.equal(target?.search.tab, 'paid-subscription-value')
  assert.equal(target?.search.subscription_id, 11)
  assert.deepEqual(target?.search.user_ids, filters.user_ids)
  assert.deepEqual(target?.search.plan_ids, filters.plan_ids)
})

test('paid subscription value subscription target falls back to user and plan filters', () => {
  const target = buildAdminAnalyticsDrilldown(filters, {
    kind: 'paid_subscription_value_subscription',
    user_id: 3,
    plan_id: 4,
  })

  assert.equal(target?.to, '/admin-analytics')
  assert.equal(target?.search.tab, 'paid-subscription-value')
  assert.equal(target?.search.subscription_id, undefined)
  assert.deepEqual(target?.search.user_ids, [3])
  assert.deepEqual(target?.search.plan_ids, [4])
})

test('paid subscription value subscription fallback clears stale subscription id', () => {
  const base = buildAdminAnalyticsCanonicalFilters({
    subscription_id: 999,
  })
  const target = buildAdminAnalyticsDrilldown(base, {
    kind: 'paid_subscription_value_subscription',
    user_id: 3,
    plan_id: 4,
  })

  assert.equal(target?.to, '/admin-analytics')
  assert.equal(target?.search.tab, 'paid-subscription-value')
  assert.equal(target?.search.subscription_id, undefined)
  assert.deepEqual(target?.search.user_ids, [3])
  assert.deepEqual(target?.search.plan_ids, [4])
})

test('paid subscription value user target does not create a route navigation', () => {
  const base = buildAdminAnalyticsCanonicalFilters({
    subscription_id: 999,
  })
  const target = buildAdminAnalyticsDrilldown(base, {
    kind: 'paid_subscription_value_user',
    user_id: 3,
  })

  assert.equal(target, null)
})



test('paid subscription value subscription target replaces stale subscription id', () => {
  const base = buildAdminAnalyticsCanonicalFilters({
    subscription_id: 999,
  })
  const target = buildAdminAnalyticsDrilldown(base, {
    kind: 'paid_subscription_value_subscription',
    subscription_id: 123,
  })

  assert.equal(target?.to, '/admin-analytics')
  assert.equal(target?.search.tab, 'paid-subscription-value')
  assert.equal(target?.search.subscription_id, 123)
})

test('invitation paid invitee target does not create a route navigation', () => {
  const base = buildAdminAnalyticsCanonicalFilters({
    tab: 'invitation-paid-subscriptions',
    snapshot_at: 123,
    currency: 'CNY',
    excluded_mode: 'include_excluded',
    plan_ids: ['4', '2'],
    sources: ['order'],
  })
  const target = buildAdminAnalyticsDrilldown(base, {
    kind: 'invitation_paid_invitee',
    inviter_id: 7,
    invitee_id: 8,
  })

  assert.equal(target, null)
})

test('invitation paid inviter target writes inviter filter', () => {
  const target = buildAdminAnalyticsDrilldown(filters, {
    kind: 'invitation_paid_inviter',
    inviter_id: 7,
  })

  assert.equal(target?.to, '/admin-analytics')
  assert.equal(target?.search.tab, 'invitation-paid-subscriptions')
  assert.equal(target?.search.inviter_id, 7)
})
