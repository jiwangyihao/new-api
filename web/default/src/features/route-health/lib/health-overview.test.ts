import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { RouteHealthRow } from '../types'
import {
  groupRouteHealth,
  selectChannelHealth,
  routeHasObservations,
} from './health-overview'

const route = (overrides: Partial<RouteHealthRow> = {}): RouteHealthRow => ({
  id: '1/model/chat',
  channel_id: 1,
  channel_name: 'primary',
  channel_status: 1,
  models: ['alias'],
  upstream_model: 'model',
  operation: '/v1/chat/completions',
  state: 'unobserved',
  ready_credentials: 1,
  total_credentials: 1,
  resources: [
    {
      scope: 'route',
      credential_index: 0,
      state: 'unobserved',
      successes: 0,
      failures: 0,
      consecutive_failures: 0,
      inflight: 0,
      opened_until: 0,
      next_probe: 0,
      probe: false,
      recovery_successes: 0,
      degraded: false,
    },
  ],
  ...overrides,
})
const filters = {
  search: '',
  view: 'all' as const,
  showManuallyDisabled: false,
}

describe('channel health overview aggregation', () => {
  test('merges aliases and operations without conflating channels or mutating input', () => {
    const chat = route()
    const rows = [
      chat,
      route({
        id: '1/model/responses',
        operation: '/v1/responses',
        models: ['model', 'alias'],
      }),
      route({ id: '2/model/chat', channel_id: 2 }),
      chat,
    ]
    const before = structuredClone(rows)
    const groups = groupRouteHealth(rows)
    assert.deepEqual(
      groups.map((group) => [group.id, group.models.length]),
      [
        [1, 1],
        [2, 1],
      ]
    )
    assert.deepEqual(groups[0].models[0].aliases, ['alias', 'model'])
    assert.deepEqual(
      groups[0].models[0].routes.map((row) => row.operation),
      ['/v1/chat/completions', '/v1/responses']
    )
    assert.deepEqual(rows, before)
  })

  test('manual disable remains hidden during searches and returns only with explicit opt-in', () => {
    const disabled = route({
      id: '2/model/chat',
      channel_id: 2,
      channel_name: 'disabled',
      channel_status: 2,
      state: 'manually_disabled',
    })
    const channels = groupRouteHealth([route(), disabled])
    assert.deepEqual(
      selectChannelHealth(channels, filters).map((item) => item.channel.id),
      [1]
    )
    assert.deepEqual(
      selectChannelHealth(channels, { ...filters, search: 'disabled' }),
      []
    )
    assert.deepEqual(
      selectChannelHealth(channels, {
        ...filters,
        search: 'disabled',
        showManuallyDisabled: true,
      }).map((item) => item.channel.id),
      [2]
    )
    assert.deepEqual(
      selectChannelHealth(channels, filters).map((item) => item.channel.id),
      [1]
    )
  })

  test('attention prioritizes failing models and excludes separate unobserved models', () => {
    const failing = route({
      id: '2/failing/responses',
      channel_id: 2,
      upstream_model: 'failing',
      models: ['failing'],
      state: 'cooling',
    })
    const unknown = route({
      id: '2/unknown/chat',
      channel_id: 2,
      upstream_model: 'unknown',
      models: ['unknown'],
    })
    const channels = groupRouteHealth([route(), unknown, failing])
    assert.deepEqual(
      channels.map((channel) => channel.id),
      [2, 1]
    )
    assert.equal(channels[0].attentionModels, 1)
    assert.deepEqual(
      channels[0].models.map((model) => model.name),
      ['failing', 'unknown']
    )
    const visible = selectChannelHealth(channels, {
      ...filters,
      view: 'attention',
    })
    assert.deepEqual(
      visible.map((item) => [
        item.channel.id,
        item.models.map((model) => model.name),
      ]),
      [[2, ['failing']]]
    )
  })

  test('shared successes do not mark unseen model routes observed, while shared failures stay visible', () => {
    const shared = {
      ...route().resources[0],
      scope: 'credential' as const,
      successes: 9,
      state: 'healthy' as const,
    }
    const unknown = route({ resources: [shared, ...route().resources] })
    assert.equal(routeHasObservations(unknown), false)
    assert.deepEqual(
      selectChannelHealth(groupRouteHealth([unknown]), {
        ...filters,
        view: 'unobserved',
      }).map((item) => item.models[0].name),
      ['model']
    )
    const failure = route({
      state: 'healthy',
      resources: [{ ...shared, failures: 1 }, ...route().resources],
    })
    const group = groupRouteHealth([failure])[0]
    assert.equal(group.attentionModels, 1)
    assert.equal(group.observedModels, 0)
    assert.deepEqual(
      selectChannelHealth([group], { ...filters, view: 'unobserved' }),
      []
    )
  })

  test('search spans channel names, model aliases and all operations without pagination', () => {
    const rows = [
      route(),
      route({
        id: '1/other/responses',
        upstream_model: 'other',
        models: ['friendly'],
        operation: '/v1/responses',
      }),
    ]
    const result = selectChannelHealth(groupRouteHealth(rows), {
      ...filters,
      search: 'PRIMARY friendly /responses',
    })
    assert.deepEqual(
      result.map((item) => item.models.map((model) => model.name)),
      [['other']]
    )
  })

  test('invalid mappings retain separate inventory identities', () => {
    const channels = groupRouteHealth([
      route({
        id: '1/invalid/a',
        upstream_model: '',
        models: ['a'],
        state: 'invalid_mapping',
      }),
      route({
        id: '1/invalid/b',
        upstream_model: '',
        models: ['b'],
        state: 'invalid_mapping',
      }),
      route({
        id: '1/literal/chat',
        upstream_model: '["a"]',
        models: ['literal'],
      }),
    ])
    assert.equal(channels[0].models.length, 3)
    assert.equal(channels[0].attentionModels, 2)
  })
})
