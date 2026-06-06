import assert from 'node:assert/strict'
import test from 'node:test'
import { buildAdminAnalyticsCanonicalFilters } from './filters'
import {
  adminAnalyticsTabLabelKey,
  buildAdminAnalyticsRequestDescriptors,
  fetchAdminAnalyticsDescriptor,
  warningReasons,
} from './page-contract'

test('single tabs map to one endpoint', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ tab: 'plans' })
  const descriptors = buildAdminAnalyticsRequestDescriptors(filters)
  assert.equal(descriptors.length, 1)
  assert.equal(descriptors[0].id, 'plan-distribution')
  assert.deepEqual(descriptors[0].queryKey, [
    'admin-analytics',
    'plans',
    'plan-distribution',
    filters,
  ])
})

test('usage tab maps to three endpoints', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ tab: 'usage' })
  const descriptors = buildAdminAnalyticsRequestDescriptors(filters)
  assert.deepEqual(
    descriptors.map((descriptor) => descriptor.id),
    [
      'usage-consumption/summary',
      'usage-consumption/timeseries',
      'usage-consumption/breakdown',
    ]
  )
})
test('admin analytics tab labels come from tab descriptors', () => {
  assert.equal(
    adminAnalyticsTabLabelKey('paid-subscription-value'),
    'adminAnalytics.tabs.paidSubscriptionValue'
  )
  assert.equal(
    adminAnalyticsTabLabelKey('invitation-paid-subscriptions'),
    'adminAnalytics.tabs.invitationPaidSubscriptions'
  )
})


test('paid subscription value descriptors negotiate snapshot before details', () => {
  const firstLoad = buildAdminAnalyticsCanonicalFilters({
    tab: 'paid-subscription-value',
  })
  const firstDescriptors = buildAdminAnalyticsRequestDescriptors(firstLoad)

  assert.deepEqual(
    firstDescriptors.map((descriptor) => descriptor.id),
    [
      'paid-subscription-value/summary',
      'paid-subscription-value/users',
      'paid-subscription-value/subscriptions',
      'paid-subscription-value/breakdown/plans',
      'paid-subscription-value/breakdown/sources',
    ]
  )
  assert.equal(firstDescriptors[0].enabled, true)
  assert.equal(firstDescriptors[1].enabled, false)
  assert.equal(firstDescriptors[2].enabled, false)
  assert.equal(firstDescriptors[2].includeSubscriptionID, true)

  const withSnapshot = buildAdminAnalyticsCanonicalFilters({
    tab: 'paid-subscription-value',
    snapshot_at: 123,
    subscription_id: 9,
  })
  const snapshotDescriptors = buildAdminAnalyticsRequestDescriptors(withSnapshot)
  assert.equal(snapshotDescriptors[0].enabled, true)
  assert.equal(snapshotDescriptors[1].enabled, true)
  assert.equal(snapshotDescriptors[2].enabled, true)
  assert.equal(snapshotDescriptors[2].includeSubscriptionID, true)
  assert.equal(snapshotDescriptors[2].includeSort, true)
  assert.equal(snapshotDescriptors[2].includeTimeRange, true)
})

test('invitation paid descriptors enable detail endpoints only after snapshot', () => {
  const firstLoad = buildAdminAnalyticsCanonicalFilters({
    tab: 'invitation-paid-subscriptions',
  })
  const firstDescriptors = buildAdminAnalyticsRequestDescriptors(firstLoad)

  assert.deepEqual(
    firstDescriptors.map((descriptor) => descriptor.id),
    [
      'invitation-paid-subscriptions/summary',
      'invitation-paid-subscriptions/inviters',
      'invitation-paid-subscriptions/invitees',
      'invitation-paid-subscriptions/subscriptions',
    ]
  )
  assert.equal(firstDescriptors[0].enabled, true)
  assert.equal(firstDescriptors[1].enabled, false)
  assert.equal(firstDescriptors[2].enabled, false)
  assert.equal(firstDescriptors[3].includeSubscriptionID, true)

  const withSnapshot = buildAdminAnalyticsCanonicalFilters({
    tab: 'invitation-paid-subscriptions',
    snapshot_at: 123,
    time_range_explicit: true,
  })
  const snapshotDescriptors = buildAdminAnalyticsRequestDescriptors(withSnapshot)
  assert.equal(snapshotDescriptors[0].enabled, true)
  assert.equal(snapshotDescriptors[1].enabled, true)
  assert.equal(snapshotDescriptors[2].enabled, true)
  assert.equal(snapshotDescriptors[3].enabled, true)
  assert.equal(snapshotDescriptors[3].includeSubscriptionID, true)
  assert.equal(snapshotDescriptors[3].includeTimeRange, true)
})

test('descriptor fetcher rejects unknown descriptors instead of falling back to overview', async () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ tab: 'overview' })

  await assert.rejects(
    () => fetchAdminAnalyticsDescriptor({ id: 'missing' } as never, filters),
    /Unknown admin analytics descriptor/
  )
})

test('descriptor options participate in request parameter construction', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'paid-subscription-value',
    snapshot_at: 123,
    subscription_id: 9,
    sort_by: 'recognized_remaining_value',
  })
  const descriptor = buildAdminAnalyticsRequestDescriptors(filters).find(
    (item) => item.id === 'paid-subscription-value/subscriptions'
  )

  assert.ok(descriptor)
  const params = descriptor.buildParams(filters)
  assert.equal(params.get('subscription_id'), '9')
  assert.equal(params.get('sort_by'), 'recognized_remaining_value')
})

test('warning reasons are stable and sorted', () => {
  assert.deepEqual(
    warningReasons([{ reason: 'b' }, { reason: 'a' }, { reason: 'b' }]),
    ['a', 'b']
  )
})
