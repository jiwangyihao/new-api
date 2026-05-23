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

test('users target only maps whitelist search fields', () => {
  const target = buildAdminAnalyticsDrilldown(filters, {
    kind: 'admin_users',
    user_id: 1,
    plan_id: 2,
    inviter_id: 3,
  })
  assert.equal(target?.to, '/users')
  assert.equal(target?.search.userId, 1)
  assert.equal(target?.search.planId, 2)
  assert.equal(target?.search.inviterId, 3)
  assert.equal(target?.search.key, undefined)
})

test('users target preserves multiple user ids and omits unknown fields', () => {
  const target = buildAdminAnalyticsDrilldown(filters, {
    kind: 'admin_users',
    user_ids: [2, 1],
    token_id: 9,
    channel_id: 3,
    status: 'error',
  })

  assert.deepEqual(target?.search.userIds, [2, 1])
  assert.equal(target?.search.tokenId, undefined)
  assert.equal(target?.search.channel, undefined)
  assert.equal(target?.search.status, undefined)
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
