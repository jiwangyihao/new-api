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
import type { QueryClient } from '@tanstack/react-query'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { subscriptionQueryKeys } from '@/features/subscriptions/query-keys'
import type { Channel } from '../types'
import { channelsQueryKeys } from './channel-actions'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  invalidateChannelTokenMultiplierRelatedQueries,
  parseTokenBillingMultiplierInput,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from './channel-form'

function validFormData(
  overrides: Partial<ChannelFormValues> = {}
): ChannelFormValues {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'billing-multiplier-channel',
    key: 'sk-test',
    models: 'gpt-5.5',
    ...overrides,
  }
}

describe('channel token billing multiplier form contract', () => {
  test('defaults channel token billing multiplier to 1', () => {
    assert.equal(CHANNEL_FORM_DEFAULT_VALUES.token_billing_multiplier, 1)
  })

  test('preserves channel token billing multiplier from API channel data', () => {
    const channel = {
      id: 1,
      name: 'upstream',
      type: 1,
      models: 'gpt-5.5',
      status: 1,
      channel_info: { multi_key_mode: 'random' },
      token_billing_multiplier: 0.5,
    } as unknown as Channel

    const defaults = transformChannelToFormDefaults(channel)

    assert.equal(defaults.token_billing_multiplier, 0.5)
  })

  test('preserves explicit zero from API channel data for validation visibility', () => {
    const channel = {
      id: 1,
      name: 'upstream',
      type: 1,
      models: 'gpt-5.5',
      status: 1,
      channel_info: { multi_key_mode: 'random' },
      token_billing_multiplier: 0,
    } as unknown as Channel

    const defaults = transformChannelToFormDefaults(channel)

    assert.equal(defaults.token_billing_multiplier, 0)
  })

  test('includes channel token billing multiplier in create payload as number', () => {
    const payload = transformFormDataToCreatePayload(
      validFormData({ token_billing_multiplier: 2 })
    )

    assert.equal(payload.channel.token_billing_multiplier, 2)
    assert.equal(typeof payload.channel.token_billing_multiplier, 'number')
  })

  test('includes channel token billing multiplier in update payload as number', () => {
    const payload = transformFormDataToUpdatePayload(
      validFormData({ token_billing_multiplier: 0.5 }),
      10
    )

    assert.equal(payload.token_billing_multiplier, 0.5)
    assert.equal(typeof payload.token_billing_multiplier, 'number')
  })

  test('rejects invalid channel token billing multiplier values', () => {
    const invalidCases = [
      {
        value: 0,
        message: 'Channel token billing multiplier must be greater than 0',
      },
      {
        value: -1,
        message: 'Channel token billing multiplier must be greater than 0',
      },
      {
        value: 101,
        message:
          'Channel token billing multiplier must be less than or equal to 100',
      },
      {
        value: 'not-a-number',
        message: 'Channel token billing multiplier is required',
      },
      {
        value: '',
        message: 'Channel token billing multiplier is required',
      },
    ]

    for (const invalidCase of invalidCases) {
      const result = channelFormSchema.safeParse({
        ...validFormData(),
        token_billing_multiplier: invalidCase.value,
      })

      assert.equal(result.success, false)
      if (!result.success) {
        assert.equal(result.error.issues[0]?.message, invalidCase.message)
      }
    }
  })

  test('coerces normal number input string to number', () => {
    const result = channelFormSchema.parse({
      ...validFormData(),
      token_billing_multiplier: '2',
    })

    assert.equal(result.token_billing_multiplier, 2)
    assert.equal(typeof result.token_billing_multiplier, 'number')
  })

  test('converts token billing multiplier input events without storing NaN', () => {
    const normalValue = parseTokenBillingMultiplierInput('2', 2)

    assert.equal(normalValue, 2)
    assert.equal(typeof normalValue, 'number')
    assert.equal(parseTokenBillingMultiplierInput('', Number.NaN), '')
    assert.equal(parseTokenBillingMultiplierInput('1e', Number.NaN), '')
    assert.equal(parseTokenBillingMultiplierInput('NaN', Number.NaN), '')
  })

  test('invalidates channel token multiplier related subscription display queries', () => {
    const invalidatedQueryKeys: unknown[] = []
    const queryClient = {
      invalidateQueries: ({ queryKey }: { queryKey: unknown }) => {
        invalidatedQueryKeys.push(queryKey)
      },
    }

    invalidateChannelTokenMultiplierRelatedQueries(
      queryClient as unknown as QueryClient
    )

    assert.deepEqual(invalidatedQueryKeys, [
      channelsQueryKeys.lists(),
      subscriptionQueryKeys.walletPlans,
      subscriptionQueryKeys.homePublicPlans,
      subscriptionQueryKeys.selfSummary,
      subscriptionQueryKeys.dashboardSelfSubscriptions,
      subscriptionQueryKeys.adminPlans,
    ])
  })
})
