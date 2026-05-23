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

import type { UsageLog } from '../data/schema'
import {
  getLegacyPromptCompletionTokens,
  getLogTokenUsage,
  getLogTokenUsageColumnValue,
  getTokenNameMeta,
  shouldShowCostDetails,
} from './format'

function makeUsageLog(overrides: Partial<UsageLog>): UsageLog {
  return {
    id: 1,
    created_at: 1_700_000_000,
    type: 2,
    username: 'alice',
    model_name: 'gpt-test',
    token_name: 'test-key',
    quota: 1000,
    prompt_tokens: 0,
    completion_tokens: 0,
    user_id: 1,
    content: '',
    use_time: 1,
    is_stream: false,
    channel: 1,
    channel_name: 'test-channel',
    token_id: 1,
    ip: '',
    request_id: '',
    upstream_request_id: '',
    other: '',
    ...overrides,
  } as UsageLog
}

test('getLogTokenUsage prefers subscription consumed tokens', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })
  const other = { subscription_tokens_consumed: 80, subscription_consumed: 20 }

  assert.equal(getLogTokenUsage(log, other), 80)
})

test('getLogTokenUsage keeps explicit zero subscription tokens', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })
  const other = { subscription_tokens_consumed: 0, subscription_consumed: 20 }

  assert.equal(getLogTokenUsage(log, other), 0)
})

test('getLogTokenUsage falls back to legacy subscription_consumed', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })
  const other = { subscription_consumed: 20 }

  assert.equal(getLogTokenUsage(log, other), 20)
})

test('getLogTokenUsage keeps explicit zero legacy subscription consumed', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })
  const other = { subscription_consumed: 0 }

  assert.equal(getLogTokenUsage(log, other), 0)
})

test('getLogTokenUsage clamps negative subscription tokens to zero', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })
  const other = { subscription_tokens_consumed: -80, subscription_consumed: 20 }

  assert.equal(getLogTokenUsage(log, other), 0)
})

test('getLogTokenUsage falls back to legacy prompt and completion tokens', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })

  assert.equal(getLogTokenUsage(log, null), 15)
})

test('getLegacyPromptCompletionTokens sums prompt and completion tokens', () => {
  const log = makeUsageLog({ prompt_tokens: 10, completion_tokens: 5 })

  assert.equal(getLegacyPromptCompletionTokens(log), 15)
})

test('getLegacyPromptCompletionTokens clamps negative token fields to zero', () => {
  const log = makeUsageLog({ prompt_tokens: -10, completion_tokens: 5 })

  assert.equal(getLegacyPromptCompletionTokens(log), 5)
})

test('shouldShowCostDetails only allows admins', () => {
  assert.equal(shouldShowCostDetails(false), false)
  assert.equal(shouldShowCostDetails(true), true)
})

test('getTokenNameMeta does not expose legacy group metadata', () => {
  const other = { model_ratio: 2 }

  assert.deepEqual(getTokenNameMeta(other, false), [])
  assert.deepEqual(getTokenNameMeta(other, true), [])
})

test('getLogTokenUsageColumnValue sorts by helper result instead of quota', () => {
  const rows = [
    makeUsageLog({ quota: 100_000, prompt_tokens: 10, completion_tokens: 5 }),
    makeUsageLog({ quota: 1, prompt_tokens: 1000, completion_tokens: 0 }),
  ]

  assert.equal(getLogTokenUsageColumnValue(rows[0]), 15)
  assert.equal(getLogTokenUsageColumnValue(rows[1]), 1000)
})
