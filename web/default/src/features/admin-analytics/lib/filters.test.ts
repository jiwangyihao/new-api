import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildAdminAnalyticsApiParams,
  buildAdminAnalyticsCanonicalFilters,
} from './filters'

test('empty search defaults to overview and recent 30 days', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({}, 1_000_000)
  assert.equal(filters.tab, 'overview')
  assert.equal(filters.granularity, 'day')
  assert.equal(filters.end_timestamp, 1_000_000)
  assert.equal(filters.start_timestamp, 1_000_000 - 30 * 24 * 60 * 60)
})

test('repeated params are deduped and sorted', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    user_groups: ['vip', '', 'default', 'vip'],
    request_groups: ['api', 'web', 'api'],
    plan_ids: ['2', '1', '2'],
  })
  assert.deepEqual(filters.user_groups, ['default', 'vip'])
  assert.deepEqual(filters.request_groups, ['api', 'web'])
  assert.deepEqual(filters.plan_ids, [1, 2])
})

test('api params use repeated query params', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    user_groups: ['vip', 'default'],
    request_groups: ['api'],
  })
  const params = buildAdminAnalyticsApiParams(filters)
  assert.deepEqual(params.getAll('user_groups'), ['default', 'vip'])
  assert.deepEqual(params.getAll('request_groups'), ['api'])
  assert.equal(params.has('groups'), false)
})

test('canonical filters preserve repeated params and serialize repeated api keys', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    user_ids: ['3', 'bad', '1', '3', '0', '-2'],
    token_ids: ['10', '2', '10', ''],
    channel_ids: ['7', 'x', '5', '7'],
    plan_ids: ['4', '2', '4'],
    user_groups: ['vip', '', 'default', 'vip'],
    request_groups: ['web', 'api', 'web'],
    sources: ['system', 'unknown', 'invalid', 'admin', 'system'],
    subscription_statuses: ['active', '', 'expired', 'active'],
    user_statuses: ['2', '1', '2'],
    log_statuses: ['success', 'error', 'success'],
    grant_reasons: ['order', 'monthly_invite_entitlement', 'order'],
    business_codes: ['pro', 'basic', '', 'pro'],
    statuses: ['legacy', 'active', 'legacy'],
  })

  assert.deepEqual(filters.user_ids, [1, 3])
  assert.deepEqual(filters.token_ids, [2, 10])
  assert.deepEqual(filters.channel_ids, [5, 7])
  assert.deepEqual(filters.plan_ids, [2, 4])
  assert.deepEqual(filters.user_groups, ['default', 'vip'])
  assert.deepEqual(filters.request_groups, ['api', 'web'])
  assert.deepEqual(filters.sources, ['admin', 'system', 'unknown'])
  assert.deepEqual(filters.subscription_statuses, ['active', 'expired'])
  assert.deepEqual(filters.user_statuses, ['1', '2'])
  assert.deepEqual(filters.log_statuses, ['error', 'success'])
  assert.deepEqual(filters.grant_reasons, ['monthly_invite_entitlement', 'order'])
  assert.deepEqual(filters.business_codes, ['basic', 'pro'])
  assert.deepEqual(filters.statuses, ['active', 'legacy'])

  const params = buildAdminAnalyticsApiParams(filters)
  assert.deepEqual(params.getAll('user_ids'), ['1', '3'])
  assert.deepEqual(params.getAll('token_ids'), ['2', '10'])
  assert.deepEqual(params.getAll('channel_ids'), ['5', '7'])
  assert.deepEqual(params.getAll('plan_ids'), ['2', '4'])
  assert.deepEqual(params.getAll('user_groups'), ['default', 'vip'])
  assert.deepEqual(params.getAll('request_groups'), ['api', 'web'])
  assert.deepEqual(params.getAll('sources'), ['admin', 'system', 'unknown'])
  assert.deepEqual(params.getAll('subscription_statuses'), [
    'active',
    'expired',
  ])
  assert.deepEqual(params.getAll('user_statuses'), ['1', '2'])
  assert.deepEqual(params.getAll('log_statuses'), ['error', 'success'])
  assert.deepEqual(params.getAll('grant_reasons'), [
    'monthly_invite_entitlement',
    'order',
  ])
  assert.deepEqual(params.getAll('business_codes'), ['basic', 'pro'])
  assert.deepEqual(params.getAll('statuses'), ['active', 'legacy'])
  assert.equal(params.has('user_id'), false)
  assert.equal(params.has('token_id'), false)
  assert.equal(params.has('channel_id'), false)
  assert.equal(params.has('plan_id'), false)
})

test('usage params omit sort unless requested', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ sort_by: 'metric' })
  const params = buildAdminAnalyticsApiParams(filters, { includeUsage: true })
  assert.equal(params.has('sort_by'), false)
  assert.equal(params.get('group_by'), 'user')
})
