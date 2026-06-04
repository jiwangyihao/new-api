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
import type { SelfSubscriptionSummary } from '@/features/subscriptions/types'
import { buildSubscriptionSummaryView } from './subscription-summary'

function summary(
  overrides: Partial<SelfSubscriptionSummary> = {}
): SelfSubscriptionSummary {
  return {
    active_count: 1,
    token_limit: 1000,
    token_used: 250,
    token_remaining: 750,
    token_unlimited: false,
    concurrency_limit: 1,
    gpt_abuse_warning_limit: 5,
    gpt_abuse_warning_count: 0,
    gpt_abuse_warning_remaining: 5,
    gpt_abuse_limit_enabled: false,
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

test('formatSubscriptionSummary exposes GPT abuse warning usage', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      gpt_abuse_warning_limit: 5,
      gpt_abuse_warning_count: 2,
      gpt_abuse_warning_remaining: 3,
      gpt_abuse_limit_enabled: true,
    })
  )

  assert.equal(result.gptAbuseWarningLabel, '2 / 5')
  assert.equal(result.gptAbuseWarningRemainingLabel, '3')
  assert.equal(result.gptAbuseLimitEnabled, true)
  assert.equal(
    result.gptAbuseStatusLabelKey,
    'GPT safety warnings will pause service at the daily limit'
  )
  assert.equal(
    result.gptAbuseStatusDescriptionKey,
    'Service interruption is enabled; reaching the limit pauses GPT access until the next day.'
  )
  assert.equal(result.gptAbuseStatusTimestamp, null)
})

test('formatSubscriptionSummary marks GPT abuse disabled as observe-only', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      gpt_abuse_warning_limit: 5,
      gpt_abuse_warning_count: 2,
      gpt_abuse_warning_remaining: 3,
      gpt_abuse_limit_enabled: false,
    })
  )

  assert.equal(result.gptAbuseWarningLabel, '2 / 5')
  assert.equal(result.gptAbuseWarningRemainingLabel, '3')
  assert.equal(result.gptAbuseLimitEnabled, false)
  assert.equal(
    result.gptAbuseStatusLabelKey,
    'GPT safety warnings are observation only'
  )
  assert.equal(
    result.gptAbuseStatusDescriptionKey,
    'Warnings are counted for visibility; service is not paused automatically.'
  )
  assert.equal(result.gptAbuseStatusTimestamp, null)
})

test('formatSubscriptionSummary exposes active GPT abuse suspension recovery time', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      gpt_abuse_warning_limit: 5,
      gpt_abuse_warning_count: 5,
      gpt_abuse_warning_remaining: 0,
      gpt_abuse_suspended_until: 1_700_000_000,
      gpt_abuse_limit_enabled: true,
    })
  )

  assert.equal(result.gptAbuseWarningLabel, '5 / 5')
  assert.equal(result.gptAbuseWarningRemainingLabel, '0')
  assert.equal(result.gptAbuseSuspendedUntil, 1_700_000_000)
  assert.equal(result.gptAbuseStatusLabelKey, 'GPT service is paused')
  assert.equal(
    result.gptAbuseStatusDescriptionKey,
    'GPT service resumes at {{time}}'
  )
  assert.equal(result.gptAbuseStatusTimestamp, 1_700_000_000)
})

test('formatSubscriptionSummary keeps GPT abuse UI fields safe without active subscription', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      active_count: 0,
      gpt_abuse_warning_limit: 5,
      gpt_abuse_warning_count: 2,
      gpt_abuse_warning_remaining: 3,
      gpt_abuse_limit_enabled: true,
    })
  )

  assert.equal(result.remainingLabel, 'Subscription required')
  assert.equal(result.gptAbuseWarningLabel, '0 / 0')
  assert.equal(result.gptAbuseWarningRemainingLabel, '0')
  assert.equal(result.gptAbuseLimitEnabled, false)
  assert.equal(result.gptAbuseSuspendedUntil, null)
  assert.equal(result.gptAbuseStatusLabelKey, 'GPT safety warnings unavailable')
  assert.equal(
    result.gptAbuseStatusDescriptionKey,
    'Activate a subscription to see GPT safety warning status.'
  )
  assert.equal(result.gptAbuseStatusTimestamp, null)
})

test('formatSubscriptionSummary warns when GPT abuse limit is exhausted but not suspended', () => {
  const result = buildSubscriptionSummaryView(
    summary({
      gpt_abuse_warning_limit: 5,
      gpt_abuse_warning_count: 5,
      gpt_abuse_warning_remaining: 0,
      gpt_abuse_limit_enabled: true,
    })
  )

  assert.equal(result.gptAbuseWarningLabel, '5 / 5')
  assert.equal(result.gptAbuseWarningRemainingLabel, '0')
  assert.equal(
    result.gptAbuseStatusLabelKey,
    'GPT safety warning limit reached'
  )
  assert.equal(
    result.gptAbuseStatusDescriptionKey,
    'Service interruption is enabled; reaching the limit pauses GPT access until the next day.'
  )
  assert.equal(result.gptAbuseStatusTimestamp, null)
})

test('formatSubscriptionSummary keeps GPT abuse UI fields safe without active summary', () => {
  const result = buildSubscriptionSummaryView(undefined)

  assert.equal(result.remainingLabel, 'Subscription required')
  assert.equal(result.gptAbuseWarningLabel, '0 / 0')
  assert.equal(result.gptAbuseWarningRemainingLabel, '0')
  assert.equal(result.gptAbuseLimitEnabled, false)
  assert.equal(result.gptAbuseSuspendedUntil, null)
  assert.equal(result.gptAbuseStatusLabelKey, 'GPT safety warnings unavailable')
  assert.equal(
    result.gptAbuseStatusDescriptionKey,
    'Activate a subscription to see GPT safety warning status.'
  )
  assert.equal(result.gptAbuseStatusTimestamp, null)
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
