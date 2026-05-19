import assert from 'node:assert/strict'
import { test } from 'node:test'
import { getUserQuotaDisplayState } from './users-columns'

test('user quota display treats used quota without remaining quota as no balance', () => {
  const state = getUserQuotaDisplayState({ quota: 0, used_quota: 3659607 })

  assert.equal(state.hasQuota, false)
  assert.equal(state.remaining, 0)
  assert.equal(state.total, 0)
  assert.equal(state.percentage, 0)
})

test('user quota display keeps remaining quota as available balance', () => {
  const state = getUserQuotaDisplayState({ quota: 500000, used_quota: 250000 })

  assert.equal(state.hasQuota, true)
  assert.equal(state.remaining, 500000)
  assert.equal(state.total, 750000)
  assert.equal(state.percentage, (500000 / 750000) * 100)
})
