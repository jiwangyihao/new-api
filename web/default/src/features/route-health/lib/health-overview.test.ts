import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { RouteHealthAttempt, RouteHealthRow } from '../types'
import {
  groupRouteHealth,
  selectChannelHealth,
  routeHasObservations,
  routeNeedsAttention,
} from './health-overview'

const route = (overrides: Partial<RouteHealthRow> = {}): RouteHealthRow => ({
  id: '1/model',
  channel_id: 1,
  channel_name: 'primary',
  channel_status: 1,
  models: ['alias'],
  upstream_model: 'model',
  state: 'unobserved',
  ready_credentials: 1,
  total_credentials: 1,
  resources: [
    {
      scope: 'model',
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
      credential_invalid: false,
      last_failure_at: 0,
      last_failure_reason: '',
    },
  ],
  attempts: [],
  ...overrides,
})
const attempt = (
  overrides: Partial<RouteHealthAttempt> = {}
): RouteHealthAttempt => ({
  request_id: 'request-1',
  index: 1,
  channel_id: 1,
  mapped_model: 'model',
  operation: '/v1/chat/completions',
  credential_index: 1,
  verdict: 'success',
  scope: 'model',
  reason: 'upstream_completed',
  status_code: 200,
  started_at: 1700000000,
  completed_at: 1700000001,
  duration_ms: 1000,
  probe: false,
  ...overrides,
})
const filters = {
  search: '',
  view: 'all' as const,
  showManuallyDisabled: false,
}

describe('channel health overview aggregation', () => {
  test('groups unique model rows without conflating channels, counts, or aliases', () => {
    const model = route({
      models: ['model', 'alias', 'alias'],
      attempts: [
        attempt(),
        attempt({ request_id: 'request-2', operation: '/v1/responses' }),
      ],
      resources: [{ ...route().resources[0], state: 'healthy', successes: 2 }],
    })
    const rows = [model, route({ id: '2/model', channel_id: 2 })]
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
    assert.deepEqual(groups[0].models[0].row.attempts, model.attempts)
    assert.equal(groups[0].models[0].row.resources[0].successes, 2)
    assert.equal(groups[0].models[0].row.ready_credentials, 1)
    assert.equal(groups[0].models[0].row.total_credentials, 1)
    assert.deepEqual(rows, before)
  })

  test('successful attempts with a completion reason do not require attention', () => {
    const successful = route({
      state: 'healthy',
      resources: [{ ...route().resources[0], state: 'healthy', successes: 1 }],
      attempts: [attempt({ reason: 'upstream_completed' })],
    })
    assert.equal(routeNeedsAttention(successful), false)
    assert.equal(groupRouteHealth([successful])[0].attentionModels, 0)
    assert.deepEqual(
      selectChannelHealth(groupRouteHealth([successful]), {
        ...filters,
        view: 'attention',
      }),
      []
    )
  })

  test('ignored cancellation and business rejection are not upstream failures', () => {
    const ignored = route({
      attempts: [
        attempt({
          verdict: 'ignored',
          reason: 'client_cancelled',
          status_code: 499,
        }),
        attempt({
          request_id: 'request-2',
          verdict: 'ignored',
          reason: 'business_rejection',
          status_code: 400,
        }),
      ],
    })
    assert.equal(routeNeedsAttention(ignored), false)
    assert.equal(routeHasObservations(ignored), false)
    assert.equal(groupRouteHealth([ignored])[0].attentionModels, 0)
  })

  test('credential isolation needs attention without overriding available model state', () => {
    const available = route({
      state: 'healthy',
      total_credentials: 2,
      resources: [
        { ...route().resources[0], state: 'healthy', successes: 1 },
        {
          ...route().resources[0],
          scope: 'credential',
          credential_index: 2,
          state: 'credential_invalid',
          credential_invalid: true,
        },
      ],
    })
    const model = groupRouteHealth([available])[0].models[0]
    assert.equal(model.attention, true)
    assert.equal(model.row.state, 'healthy')
    assert.equal(model.row.ready_credentials, 1)
    assert.equal(model.row.total_credentials, 2)
  })

  test('degraded resources and real failures require attention', () => {
    assert.equal(
      routeNeedsAttention(
        route({ resources: [{ ...route().resources[0], degraded: true }] })
      ),
      true
    )
    assert.equal(
      routeNeedsAttention(
        route({
          attempts: [
            attempt({
              verdict: 'failure',
              reason: 'upstream_timeout',
              status_code: 504,
            }),
          ],
        })
      ),
      true
    )
  })

  test('only model window samples, inflight, or probes establish observations', () => {
    for (const sample of [
      { successes: 1 },
      { failures: 1 },
      { inflight: 1 },
      { probe: true },
    ]) {
      assert.equal(
        routeHasObservations(
          route({ resources: [{ ...route().resources[0], ...sample }] })
        ),
        true
      )
    }
    assert.equal(routeHasObservations(route({ attempts: [attempt()] })), false)
  })

  test('manual disable remains hidden during searches and returns only with explicit opt-in', () => {
    const disabled = route({
      id: '2/model',
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
      id: '2/failing',
      channel_id: 2,
      upstream_model: 'failing',
      models: ['failing'],
      state: 'cooling',
    })
    const unknown = route({
      id: '2/unknown',
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
      credential_index: 1,
      successes: 9,
      state: 'healthy' as const,
    }
    const unknown = route({
      resources: [
        shared,
        { ...shared, scope: 'channel', credential_index: 0 },
        ...route().resources,
      ],
    })
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
        id: '1/other',
        upstream_model: 'other',
        models: ['friendly'],
        attempts: [
          attempt({
            mapped_model: 'other',
            operation: '/v1/responses',
            reason: 'diagnostic_marker',
          }),
        ],
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
    assert.equal(
      selectChannelHealth(groupRouteHealth(rows), {
        ...filters,
        search: 'diagnostic_marker',
      })[0].models[0].name,
      'other'
    )
    assert.equal(
      selectChannelHealth(groupRouteHealth(rows), {
        ...filters,
        search: '1 other',
      })[0].models[0].name,
      'other'
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
        id: '1/literal',
        upstream_model: '["a"]',
        models: ['literal'],
      }),
    ])
    assert.equal(channels[0].models.length, 3)
    assert.equal(channels[0].attentionModels, 2)
  })
})
