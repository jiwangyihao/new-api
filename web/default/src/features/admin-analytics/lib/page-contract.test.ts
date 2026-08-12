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

test('conversion tab loads summary and subscription history endpoints', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'conversion',
    snapshot_at: 123,
    subscription_statuses: ['converted', 'expired'],
  })
  const descriptors = buildAdminAnalyticsRequestDescriptors(filters)

  assert.deepEqual(
    descriptors.map((descriptor) => descriptor.id),
    ['subscription-conversion', 'drilldown/subscriptions']
  )
  assert.equal(descriptors[1].enabled, true)
  assert.equal(descriptors[1].includeSort, true)
  assert.deepEqual(descriptors[1].queryKey, [
    'admin-analytics',
    'conversion',
    'drilldown/subscriptions',
    filters,
  ])
  assert.deepEqual(
    descriptors[1].buildParams(filters).getAll('subscription_statuses'),
    ['converted', 'expired']
  )
})

test('conversion history defaults to converted and in-grace expired statuses', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({ tab: 'conversion' })
  const history = buildAdminAnalyticsRequestDescriptors(filters)[1]

  assert.ok(history)
  assert.deepEqual(history.buildParams(filters).getAll('subscription_statuses'), [
    'converted',
    'expired',
  ])
  assert.deepEqual(history.queryKey, [
    'admin-analytics',
    'conversion',
    'drilldown/subscriptions',
    { ...filters, subscription_statuses: ['converted', 'expired'] },
  ])
})

test('explicit conversion history statuses override the lifecycle default', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'conversion',
    subscription_statuses: ['expired'],
  })
  const history = buildAdminAnalyticsRequestDescriptors(filters)[1]

  assert.ok(history)
  assert.deepEqual(history.buildParams(filters).getAll('subscription_statuses'), [
    'expired',
  ])
  assert.deepEqual(history.queryKey, [
    'admin-analytics',
    'conversion',
    'drilldown/subscriptions',
    filters,
  ])
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
  const snapshotDescriptors =
    buildAdminAnalyticsRequestDescriptors(withSnapshot)
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
  const snapshotDescriptors =
    buildAdminAnalyticsRequestDescriptors(withSnapshot)
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
    currency: 'CNY',
  })
  const descriptor = buildAdminAnalyticsRequestDescriptors(filters).find(
    (item) => item.id === 'paid-subscription-value/subscriptions'
  )

  assert.ok(descriptor)
  const params = descriptor.buildParams(filters)
  assert.equal(params.get('subscription_id'), '9')
  assert.equal(params.get('sort_by'), 'recognized_remaining_value')
})

test('paid descriptors only serialize sort fields accepted by each endpoint', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'paid-subscription-value',
    snapshot_at: 123,
    sort_by: 'user_id',
  })
  const descriptors = buildAdminAnalyticsRequestDescriptors(filters)
  const users = descriptors.find(
    (item) => item.id === 'paid-subscription-value/users'
  )
  const plans = descriptors.find(
    (item) => item.id === 'paid-subscription-value/breakdown/plans'
  )

  assert.ok(users)
  assert.ok(plans)
  assert.equal(users.buildParams(filters).get('sort_by'), 'user_id')
  assert.equal(plans.buildParams(filters).has('sort_by'), false)
})

test('single endpoint descriptors only serialize sort fields where backend accepts them', () => {
  const usersFilters = buildAdminAnalyticsCanonicalFilters({
    tab: 'users',
    sort_by: 'user_id',
  })
  const users = buildAdminAnalyticsRequestDescriptors(usersFilters)[0]
  assert.equal(users.id, 'user-lifecycle')
  assert.equal(users.buildParams(usersFilters).has('sort_by'), false)

  const conversionFilters = buildAdminAnalyticsCanonicalFilters({
    tab: 'conversion',
    sort_by: 'user_id',
  })
  const conversion = buildAdminAnalyticsRequestDescriptors(conversionFilters)[0]
  assert.equal(conversion.id, 'subscription-conversion')
  assert.equal(conversion.buildParams(conversionFilters).has('sort_by'), false)

  const plansFilters = buildAdminAnalyticsCanonicalFilters({
    tab: 'plans',
    sort_by: 'user_count',
  })
  const plans = buildAdminAnalyticsRequestDescriptors(plansFilters)[0]
  assert.equal(plans.id, 'plan-distribution')
  assert.equal(plans.buildParams(plansFilters).get('sort_by'), 'user_count')
})

test('invitation paid aggregate descriptors do not serialize subscription id', () => {
  const filters = buildAdminAnalyticsCanonicalFilters({
    tab: 'invitation-paid-subscriptions',
    snapshot_at: 123,
    subscription_id: 9,
  })
  const descriptors = buildAdminAnalyticsRequestDescriptors(filters)

  for (const id of [
    'invitation-paid-subscriptions/summary',
    'invitation-paid-subscriptions/inviters',
    'invitation-paid-subscriptions/invitees',
  ]) {
    const descriptor = descriptors.find((item) => item.id === id)
    assert.ok(descriptor)
    assert.equal(descriptor.buildParams(filters).has('subscription_id'), false)
  }

  const subscriptions = descriptors.find(
    (item) => item.id === 'invitation-paid-subscriptions/subscriptions'
  )
  assert.ok(subscriptions)
  assert.equal(subscriptions.buildParams(filters).get('subscription_id'), '9')
})

test('warning reasons are stable and sorted', () => {
  assert.deepEqual(
    warningReasons([{ reason: 'b' }, { reason: 'a' }, { reason: 'b' }]),
    ['a', 'b']
  )
})
