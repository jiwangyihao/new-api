import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { groupRouteHealth, selectChannelHealth } from './health-overview'
import type { RouteHealthRow } from '../types'

const route = (overrides: Partial<RouteHealthRow> = {}): RouteHealthRow => ({
  id: '1/model/chat', channel_id: 1, channel_name: 'primary', channel_status: 1,
  models: ['alias'], upstream_model: 'model', operation: '/v1/chat/completions',
  state: 'unobserved', ready_credentials: 1, total_credentials: 1,
  resources: [{ scope: 'route', credential_index: 0, state: 'unobserved', successes: 0, failures: 0,
    consecutive_failures: 0, inflight: 0, opened_until: 0, next_probe: 0, probe: false, recovery_successes: 0, degraded: false }],
  ...overrides,
})

describe('channel health overview aggregation', () => {
  test('groups routes by channel and upstream model without duplicating aliases', () => {
    const groups = groupRouteHealth([
      route(),
      route({ id: '1/model/responses', operation: '/v1/responses', models: ['alias', 'model'] }),
    ])
    assert.equal(groups.length, 1)
    assert.equal(groups[0].models.length, 1)
    assert.deepEqual(groups[0].models[0].aliases, ['alias', 'model'])
    assert.equal(groups[0].models[0].routes.length, 2)
  })

  test('hides manually disabled channels by default and restores them explicitly', () => {
    const disabled = route({ id: '2/model/chat', channel_id: 2, channel_name: 'disabled', channel_status: 2, state: 'manually_disabled' })
    const channels = groupRouteHealth([route(), disabled])
    assert.equal(selectChannelHealth(channels, { search: '', view: 'all', showManuallyDisabled: false }).length, 1)
    assert.equal(selectChannelHealth(channels, { search: '', view: 'all', showManuallyDisabled: true }).length, 2)
  })

  test('attention view excludes unobserved-only models', () => {
    const attention = route({ id: '1/model/bad', state: 'cooling', resources: [{ ...route().resources[0], state: 'cooling' }] })
    const channels = groupRouteHealth([route(), attention])
    const visible = selectChannelHealth(channels, { search: '', view: 'attention', showManuallyDisabled: false })
    assert.equal(visible.length, 1)
    assert.equal(visible[0].models[0].name, 'model')
  })
})
