import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildAdminAnalyticsApiParams,
  buildAdminAnalyticsCanonicalFilters,
  enableAdminAnalyticsAllRows,
  enableAdminAnalyticsPagedRows,
  switchAdminAnalyticsTab,
} from './filters'

const deprecatedUserDimensionParam = 'user_' + 'groups'
const deprecatedRequestDimensionParam = 'request_' + 'groups'

test('empty search defaults to overview and recent 30 days', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({}, 1_000_000)
  assert.equal(filters.tab, 'overview')
  assert.equal(filters.granularity, 'day')
  assert.equal(filters.end_timestamp, 1_000_000)
  assert.equal(filters.start_timestamp, 1_000_000 - 30 * 24 * 60 * 60)
})

test('legacy analytics filters keep conservative list defaults', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ tab: 'overview' })

  assert.equal(filters.limit, 20)
  assert.equal(filters.top_n, 20)
  assert.equal(buildAdminAnalyticsApiParams(filters).get('limit'), '20')
  assert.equal(
    buildAdminAnalyticsApiParams(filters, { includeUsage: true }).get('top_n'),
    '20'
  )
})

test('paid subscription analytics filters default to explicit all rows', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'paid-subscription-value',
  })

  assert.equal(filters.limit, 0)
  assert.equal(filters.top_n, 0)
  assert.equal(buildAdminAnalyticsApiParams(filters).get('limit'), '0')
})

test('new paid subscription analytics tabs are accepted', () => {
  assert.equal(
    buildAdminAnalyticsCanonicalFilters({ tab: 'paid-subscription-value' }).tab,
    'paid-subscription-value'
  )
  assert.equal(
    buildAdminAnalyticsCanonicalFilters({
      tab: 'invitation-paid-subscriptions',
    }).tab,
    'invitation-paid-subscriptions'
  )
})

test('excluded mode accepts only the three supported states', () => {
  assert.equal(
    buildAdminAnalyticsCanonicalFilters({ excluded_mode: 'included_only' })
      .excluded_mode,
    'included_only'
  )
  assert.equal(
    buildAdminAnalyticsCanonicalFilters({ excluded_mode: 'include_excluded' })
      .excluded_mode,
    'include_excluded'
  )
  assert.equal(
    buildAdminAnalyticsCanonicalFilters({ excluded_mode: 'excluded_only' })
      .excluded_mode,
    'excluded_only'
  )
  assert.equal(
    buildAdminAnalyticsCanonicalFilters({ excluded_mode: 'all' }).excluded_mode,
    'included_only'
  )
})

test('canonical filters preserve paid subscription analytics fields', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'invitation-paid-subscriptions',
    snapshot_at: '0',
    currency: 'CNY',
    excluded_mode: 'include_excluded',
    active_only: 'true',
    time_range_explicit: 'true',
    inviter_id: '7',
    invitee_id: '8',
    subscription_id: '9',
  })

  assert.equal(filters.snapshot_at, 0)
  assert.equal(filters.currency, 'CNY')
  assert.equal(filters.excluded_mode, 'include_excluded')
  assert.equal(filters.active_only, true)
  assert.equal(filters.time_range_explicit, true)
  assert.equal(filters.inviter_id, 7)
  assert.equal(filters.invitee_id, 8)
  assert.equal(filters.subscription_id, 9)
})

test('paid subscription analytics params keep snapshot zero and shared filters', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'invitation-paid-subscriptions',
    snapshot_at: '0',
    currency: 'USD',
    excluded_mode: 'excluded_only',
    active_only: 'true',
    inviter_id: '7',
    invitee_id: '8',
    subscription_id: '9',
    user_ids: ['3', '1'],
    plan_ids: ['4', '2'],
  })
  const params = buildAdminAnalyticsApiParams(filters, {
    includeSubscriptionID: true,
  })

  assert.equal(params.get('snapshot_at'), '0')
  assert.equal(params.get('currency'), 'USD')
  assert.equal(params.get('excluded_mode'), 'excluded_only')
  assert.equal(params.get('active_only'), 'true')
  assert.equal(params.get('inviter_id'), '7')
  assert.equal(params.get('invitee_id'), '8')
  assert.equal(params.get('subscription_id'), '9')
  assert.deepEqual(params.getAll('user_ids'), ['1', '3'])
  assert.deepEqual(params.getAll('plan_ids'), ['2', '4'])
})

test('subscription id is serialized only when descriptor opts in', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'paid-subscription-value',
    snapshot_at: 123,
    subscription_id: 9,
  })

  assert.equal(
    buildAdminAnalyticsApiParams(filters).has('subscription_id'),
    false
  )
  assert.equal(
    buildAdminAnalyticsApiParams(filters, { includeSubscriptionID: true }).get(
      'subscription_id'
    ),
    '9'
  )
})

test('new paid subscription analytics tabs do not send default range until explicit', () => {
  const filters = buildAdminAnalyticsCanonicalFilters(
    { tab: 'paid-subscription-value' },
    1_000_000
  )

  assert.equal(filters.start_timestamp, 1_000_000 - 30 * 24 * 60 * 60)
  assert.equal(filters.end_timestamp, 1_000_000)
  assert.equal(filters.time_range_explicit, false)

  const params = buildAdminAnalyticsApiParams(filters)
  assert.equal(params.has('start_timestamp'), false)
  assert.equal(params.has('end_timestamp'), false)

  const explicit = buildAdminAnalyticsCanonicalFilters(
    { tab: 'paid-subscription-value', time_range_explicit: 'true' },
    1_000_000
  )
  const explicitParams = buildAdminAnalyticsApiParams(explicit)
  assert.equal(
    explicitParams.get('start_timestamp'),
    String(1_000_000 - 30 * 24 * 60 * 60)
  )
  assert.equal(explicitParams.get('end_timestamp'), '1000000')
})

test('analytics filters support unbounded list requests', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    limit: 'all',
    top_n: '0',
  })

  assert.equal(filters.limit, 0)
  assert.equal(filters.top_n, 0)
  assert.equal(buildAdminAnalyticsApiParams(filters).get('limit'), '0')
  assert.equal(
    buildAdminAnalyticsApiParams(filters, { includeUsage: true }).get('top_n'),
    '0'
  )
})

test('switching between paid subscription analytics tabs clears stale implicit time range', () => {
  const legacyFilters = buildAdminAnalyticsCanonicalFilters(
    { tab: 'overview' },
    1_000_000
  )

  const paidFilters = switchAdminAnalyticsTab(
    legacyFilters,
    'paid-subscription-value'
  )
  const paidParams = buildAdminAnalyticsApiParams(paidFilters)

  assert.equal(paidFilters.tab, 'paid-subscription-value')
  assert.equal(paidFilters.time_range_explicit, false)
  assert.equal(paidFilters.snapshot_at, undefined)
  assert.equal(paidParams.has('start_timestamp'), false)
  assert.equal(paidParams.has('end_timestamp'), false)

  const explicitPaidFilters = buildAdminAnalyticsCanonicalFilters(
    {
      tab: 'paid-subscription-value',
      snapshot_at: 123,
      time_range_explicit: 'true',
      start_timestamp: 10,
      end_timestamp: 20,
      subscription_id: 9,
      inviter_id: 7,
      invitee_id: 8,
    },
    1_000_000
  )

  const inviteFilters = switchAdminAnalyticsTab(
    explicitPaidFilters,
    'invitation-paid-subscriptions'
  )
  const inviteParams = buildAdminAnalyticsApiParams(inviteFilters)

  assert.equal(inviteFilters.tab, 'invitation-paid-subscriptions')
  assert.equal(inviteFilters.snapshot_at, undefined)
  assert.equal(inviteFilters.subscription_id, undefined)
  assert.equal(inviteFilters.inviter_id, undefined)
  assert.equal(inviteFilters.invitee_id, undefined)
  assert.equal(inviteFilters.time_range_explicit, false)
  assert.equal(inviteParams.has('start_timestamp'), false)
  assert.equal(inviteParams.has('end_timestamp'), false)
})

test('switching tabs resets list limits for the target tab family', () => {
  const legacyFilters = buildAdminAnalyticsCanonicalFilters({ tab: 'overview' })
  const paidFilters = switchAdminAnalyticsTab(
    legacyFilters,
    'paid-subscription-value'
  )
  assert.equal(paidFilters.limit, 0)
  assert.equal(paidFilters.top_n, 0)

  const nextLegacyFilters = switchAdminAnalyticsTab(paidFilters, 'overview')
  assert.equal(nextLegacyFilters.limit, 20)
  assert.equal(nextLegacyFilters.top_n, 20)
})

test('paid analytics list controls can switch between all rows and paged rows', () => {
  const paged = buildAdminAnalyticsCanonicalFilters({
    tab: 'paid-subscription-value',
    limit: '25',
    top_n: '25',
    offset: '50',
  })

  const allRows = enableAdminAnalyticsAllRows(paged)
  assert.equal(allRows.limit, 0)
  assert.equal(allRows.top_n, 0)
  assert.equal(allRows.offset, 0)

  const nextPaged = enableAdminAnalyticsPagedRows(allRows, 30)
  assert.equal(nextPaged.limit, 30)
  assert.equal(nextPaged.top_n, 30)
  assert.equal(nextPaged.offset, 0)
})

test('legacy tabs still send their default recent range', () => {
  const filters = buildAdminAnalyticsCanonicalFilters(
    { tab: 'overview' },
    1_000_000
  )
  const params = buildAdminAnalyticsApiParams(filters)

  assert.equal(
    params.get('start_timestamp'),
    String(1_000_000 - 30 * 24 * 60 * 60)
  )
  assert.equal(params.get('end_timestamp'), '1000000')
})

test('deprecated analytics params are ignored while repeated params are deduped and sorted', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    [deprecatedUserDimensionParam]: ['vip', '', 'default', 'vip'],
    [deprecatedRequestDimensionParam]: ['api', 'web', 'api'],
    plan_ids: ['2', '1', '2'],
  })
  assert.deepEqual(filters.plan_ids, [1, 2])
  assert.equal(deprecatedUserDimensionParam in filters, false)
  assert.equal(deprecatedRequestDimensionParam in filters, false)
})

test('canonical filters preserve repeated params and serialize repeated api keys', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    user_ids: ['3', 'bad', '1', '3', '0', '-2'],
    token_ids: ['10', '2', '10', ''],
    channel_ids: ['7', 'x', '5', '7'],
    plan_ids: ['4', '2', '4'],
    [deprecatedUserDimensionParam]: ['vip', '', 'default', 'vip'],
    [deprecatedRequestDimensionParam]: ['web', 'api', 'web'],
    sources: ['system', 'unknown', 'invalid', 'admin', 'system'],
    subscription_statuses: ['active', '', 'expired', 'active'],
    user_statuses: ['enabled', 'disabled', 'enabled'],
    log_statuses: ['success', 'error', 'success'],
    grant_reasons: ['order', 'monthly_invite_entitlement', 'order'],
    business_codes: ['pro', 'basic', '', 'pro'],
    statuses: ['legacy', 'active', 'legacy'],
  })

  assert.deepEqual(filters.user_ids, [1, 3])
  assert.deepEqual(filters.token_ids, [2, 10])
  assert.deepEqual(filters.channel_ids, [5, 7])
  assert.deepEqual(filters.plan_ids, [2, 4])
  assert.equal(deprecatedUserDimensionParam in filters, false)
  assert.equal(deprecatedRequestDimensionParam in filters, false)
  assert.deepEqual(filters.sources, ['admin', 'system', 'unknown'])
  assert.deepEqual(filters.subscription_statuses, ['active', 'expired'])
  assert.deepEqual(filters.user_statuses, ['disabled', 'enabled'])
  assert.deepEqual(filters.log_statuses, ['error', 'success'])
  assert.deepEqual(filters.grant_reasons, [
    'monthly_invite_entitlement',
    'order',
  ])
  assert.deepEqual(filters.business_codes, ['basic', 'pro'])
  assert.deepEqual(filters.statuses, ['active', 'legacy'])

  const params = buildAdminAnalyticsApiParams(filters)
  assert.deepEqual(params.getAll('user_ids'), ['1', '3'])
  assert.deepEqual(params.getAll('token_ids'), ['2', '10'])
  assert.deepEqual(params.getAll('channel_ids'), ['5', '7'])
  assert.deepEqual(params.getAll('plan_ids'), ['2', '4'])
  assert.equal(params.has(deprecatedUserDimensionParam), false)
  assert.equal(params.has(deprecatedRequestDimensionParam), false)
  assert.deepEqual(params.getAll('sources'), ['admin', 'system', 'unknown'])
  assert.deepEqual(params.getAll('subscription_statuses'), [
    'active',
    'expired',
  ])
  assert.deepEqual(params.getAll('user_statuses'), ['disabled', 'enabled'])
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

test('canonical filters preserve user status enum values', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    user_statuses: ['enabled', 'disabled', 'enabled'],
  })

  assert.deepEqual(filters.user_statuses, ['disabled', 'enabled'])
  const params = buildAdminAnalyticsApiParams(filters)
  assert.deepEqual(params.getAll('user_statuses'), ['disabled', 'enabled'])
})

test('api params omit deprecated analytics params', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    [deprecatedUserDimensionParam]: ['vip', 'default'],
    [deprecatedRequestDimensionParam]: ['api'],
  })
  const params = buildAdminAnalyticsApiParams(filters)
  assert.equal(params.has(deprecatedUserDimensionParam), false)
  assert.equal(params.has(deprecatedRequestDimensionParam), false)
  assert.equal(params.has('groups'), false)
})

test('usage params omit sort unless requested', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ sort_by: 'metric' })
  const params = buildAdminAnalyticsApiParams(filters, { includeUsage: true })
  assert.equal(params.has('sort_by'), false)
  assert.equal(params.get('group_by'), 'user')
})
