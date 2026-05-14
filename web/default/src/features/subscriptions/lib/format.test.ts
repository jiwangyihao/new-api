import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { formatConcurrencyLimit, formatTokenLimit } from './format'

const t = (key: string, values?: Record<string, unknown>) =>
  key.replace(/{{(\w+)}}/g, (_match, name: string) => String(values?.[name] ?? ''))

describe('subscription distributor format helpers', () => {
  test('formats zero monthly token limit as unlimited tokens', () => {
    assert.equal(formatTokenLimit(0, t), 'Unlimited tokens')
  })

  test('formats billion-scale monthly token limits compactly', () => {
    assert.equal(formatTokenLimit(1_000_000_000, t), '1B tokens')
    assert.equal(formatTokenLimit(2_500_000_000, t), '2.5B tokens')
  })

  test('formats concurrency limit with unlimited fallback', () => {
    assert.equal(formatConcurrencyLimit(0, t), 'Unlimited concurrency')
    assert.equal(formatConcurrencyLimit(5, t), '5 concurrent requests')
  })
})
