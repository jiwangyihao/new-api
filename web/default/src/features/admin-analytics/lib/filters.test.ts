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

test('usage params omit sort unless requested', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ sort_by: 'metric' })
  const params = buildAdminAnalyticsApiParams(filters, { includeUsage: true })
  assert.equal(params.has('sort_by'), false)
  assert.equal(params.get('group_by'), 'user')
})
