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
import { test } from 'node:test'
import { buildSubscriptionSummaryView } from './subscription-summary'
import type { SelfSubscriptionSummary } from '@/features/subscriptions/types'

function summary(
  overrides: Partial<SelfSubscriptionSummary> = {}
): SelfSubscriptionSummary {
  return {
    active_count: 1,
    token_limit: 1000,
    token_used: 0,
    token_remaining: 1000,
    token_unlimited: false,
    concurrency_limit: 1,
    ...overrides,
  }
}

test('formatSubscriptionSummary displays finite remaining tokens', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      token_limit: 1000,
      token_used: 250,
      token_remaining: 750,
      token_unlimited: false,
      active_count: 1,
      concurrency_limit: 1,
    })
  )

  assert.equal(result.remainingLabel, '750')
  assert.equal(result.healthLevel, 'healthy')
})

test('formatSubscriptionSummary treats unlimited only when token_unlimited is true', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      token_limit: 0,
      token_used: 0,
      token_remaining: 0,
      token_unlimited: false,
      active_count: 1,
      concurrency_limit: 1,
    })
  )

  assert.notEqual(result.remainingLabel, 'Unlimited')
})

test('formatSubscriptionSummary displays Unlimited only for explicit unlimited summary', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      token_limit: 0,
      token_used: 250,
      token_remaining: 0,
      token_unlimited: true,
      active_count: 1,
      concurrency_limit: 1,
    })
  )

  assert.equal(result.remainingLabel, 'Unlimited')
  assert.equal(result.healthLevel, 'healthy')
})

test('formatSubscriptionSummary marks missing subscription as required', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      token_limit: 0,
      token_used: 0,
      token_remaining: 0,
      token_unlimited: false,
      active_count: 0,
      concurrency_limit: 0,
    })
  )

  assert.equal(result.remainingLabel, 'Subscription required')
  assert.equal(result.healthLevel, 'critical')
})

test('formatSubscriptionSummary exposes reset time when present', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      next_reset_time: 1_700_000_000,
      end_time: 1_800_000_000,
    })
  )

  assert.equal(result.timeLabelKey, 'Subscription resets at')
  assert.equal(result.timeTimestamp, 1_700_000_000)
})

test('formatSubscriptionSummary falls back to expiry time when reset time is absent', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      end_time: 1_800_000_000,
    })
  )

  assert.equal(result.timeLabelKey, 'Subscription expires at')
  assert.equal(result.timeTimestamp, 1_800_000_000)
})
