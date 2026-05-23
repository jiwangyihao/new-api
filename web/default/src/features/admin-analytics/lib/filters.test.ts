import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildAdminAnalyticsApiParams,
  buildAdminAnalyticsCanonicalFilters,
} from './filters'

const deprecatedUserGroupsParam = 'user_' + 'groups'
const deprecatedRequestGroupsParam = 'request_' + 'groups'

test('empty search defaults to overview and recent 30 days', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({}, 1_000_000)
  assert.equal(filters.tab, 'overview')
  assert.equal(filters.granularity, 'day')
  assert.equal(filters.end_timestamp, 1_000_000)
  assert.equal(filters.start_timestamp, 1_000_000 - 30 * 24 * 60 * 60)
})

test('business group params are ignored while repeated params are deduped and sorted', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    [deprecatedUserGroupsParam]: ['vip', '', 'default', 'vip'],
    [deprecatedRequestGroupsParam]: ['api', 'web', 'api'],
    plan_ids: ['2', '1', '2'],
  })
  assert.deepEqual(filters.plan_ids, [1, 2])
  assert.equal(deprecatedUserGroupsParam in filters, false)
  assert.equal(deprecatedRequestGroupsParam in filters, false)
})

test('api params omit deprecated business group params', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    [deprecatedUserGroupsParam]: ['vip', 'default'],
    [deprecatedRequestGroupsParam]: ['api'],
  })
  const params = buildAdminAnalyticsApiParams(filters)
  assert.equal(params.has(deprecatedUserGroupsParam), false)
  assert.equal(params.has(deprecatedRequestGroupsParam), false)
  assert.equal(params.has('groups'), false)
})


test('usage params omit sort unless requested', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ sort_by: 'metric' })
  const params = buildAdminAnalyticsApiParams(filters, { includeUsage: true })
  assert.equal(params.has('sort_by'), false)
  assert.equal(params.get('group_by'), 'user')
})
