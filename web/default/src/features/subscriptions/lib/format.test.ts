import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  formatConcurrencyLimit,
  formatFiniteTokenCount,
  formatPlanPrice,
  formatQueueCapacity,
  formatTokenLimit,
} from './format'
import { subscriptionPlanSchema } from '../types'

const t = (key: string, values?: Record<string, unknown>) =>
  key.replace(/{{(\w+)}}/g, (_match, name: string) =>
    String(values?.[name] ?? '')
  )

describe('subscription distributor format helpers', () => {
  test('formats zero monthly token limit as unlimited tokens', () => {
    assert.equal(formatTokenLimit(0, t), 'Unlimited tokens')
  })

  test('formats finite zero token counts as zero tokens', () => {
    assert.equal(formatFiniteTokenCount(0, t), '0 tokens')
  })

  test('parses discriminated channel token equivalents from plan API data', () => {
    const plan = subscriptionPlanSchema.parse({
      id: 1,
      title: 'Pro',
      price_amount: 10,
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'monthly',
      enabled: true,
      sort_order: 1,
      max_purchase_per_user: 0,
      total_amount: 0,
      channel_token_equivalents: [
        {
          kind: 'range',
          channel_type: 14,
          channel_type_name: 'Claude',
          variant_count: 2,
          min_multiplier: 1.5,
          max_multiplier: 2,
          equivalent_token_limit_min: 500_000,
          equivalent_token_limit_max: 666_666,
        },
      ],
    })

    assert.equal(plan.channel_token_equivalents[0]?.kind, 'range')
  })

  test('formats billion-scale monthly token limits compactly', () => {
    assert.equal(formatTokenLimit(1_000_000_000, t), '1B tokens')
    assert.equal(formatTokenLimit(2_500_000_000, t), '2.5B tokens')
  })

  test('formats concurrency limit with unlimited fallback', () => {
    assert.equal(formatConcurrencyLimit(0, t), 'Unlimited concurrency')
    assert.equal(formatConcurrencyLimit(5, t), '5 concurrent requests')
  })

  test('formats queue capacity with global fallback', () => {
    assert.equal(formatQueueCapacity(0, t), 'Use global queue capacity')
    assert.equal(formatQueueCapacity(12, t), '12 queued requests')
  })

  test('formats plan prices by plan currency', () => {
    assert.equal(formatPlanPrice(40, 'CNY'), '¥40.00')
    assert.equal(formatPlanPrice(40, 'USD'), '$40.00')
    assert.equal(formatPlanPrice(40, 'EUR'), 'EUR 40.00')
    assert.equal(formatPlanPrice(40, ''), '¥40.00')
  })
})
