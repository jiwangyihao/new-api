import assert from 'node:assert/strict'
import test from 'node:test'
import { buildAdminAnalyticsCanonicalFilters } from './filters'
import {
  buildAdminAnalyticsRequestDescriptors,
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

test('warning reasons are stable and sorted', () => {
  assert.deepEqual(
    warningReasons([{ reason: 'b' }, { reason: 'a' }, { reason: 'b' }]),
    ['a', 'b']
  )
})
