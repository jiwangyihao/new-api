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
import { describe, test } from 'node:test'
import type { Channel } from '../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from './channel-form'

describe('channel endpoint capabilities form contract', () => {
  test('preserves supported endpoint types from channel settings', () => {
    const channel = {
      id: 1,
      name: 'upstream',
      type: 1,
      models: 'gpt-5.5',
      group: 'default',
      status: 1,
      channel_info: {},
      settings: JSON.stringify({
        supported_endpoint_types: [
          'openai-response',
          'openai-response-compact',
        ],
      }),
    } as unknown as Channel

    const defaults = transformChannelToFormDefaults(channel)

    assert.deepEqual(defaults.supported_endpoint_types, [
      'openai-response',
      'openai-response-compact',
    ])
  })

  test('preserves and serializes the OpenAI Responses account group', () => {
    const channel = {
      id: 2,
      name: 'responses-upstream',
      type: 1,
      models: 'gpt-5.5',
      group: 'default',
      status: 1,
      channel_info: {},
      settings: JSON.stringify({ account_group: 'team-a' }),
    } as unknown as Channel

    const defaults = transformChannelToFormDefaults(channel)
    assert.equal(defaults.account_group, 'team-a')

    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'responses-upstream',
      type: 1,
      key: 'sk-test',
      models: 'gpt-5.5',
      account_group: ' team-a ',
    })

    assert.equal(
      JSON.parse(payload.channel.settings || '{}').account_group,
      'team-a'
    )
  })

  test('serializes supported endpoint types into channel settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'upstream',
      type: 1,
      key: 'sk-test',
      models: 'gpt-5.5',
      supported_endpoint_types: [
        'openai-response',
        'openai-response-compact',
        'openai-response-compact',
      ],
    })

    assert.deepEqual(
      JSON.parse(payload.channel.settings || '{}').supported_endpoint_types,
      ['openai-response', 'openai-response-compact']
    )
  })

  test('clearing the account group removes the saved routing restriction', () => {
    const payload = transformFormDataToUpdatePayload(
      {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'responses-upstream',
        models: 'gpt-5.5',
        settings: JSON.stringify({
          account_group: 'team-a',
          custom_setting: 'keep',
        }),
        account_group: '   ',
      },
      2
    )

    const settings = JSON.parse(payload.settings || '{}')
    assert.equal(Object.hasOwn(settings, 'account_group'), false)
    assert.equal(settings.custom_setting, 'keep')
  })

  test('switching away from OpenAI removes the account group setting', () => {
    const payload = transformFormDataToUpdatePayload(
      {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'other-upstream',
        type: 14,
        models: 'claude-test',
        settings: JSON.stringify({ account_group: 'team-a' }),
        account_group: 'team-a',
      },
      2
    )

    assert.equal(
      Object.hasOwn(JSON.parse(payload.settings || '{}'), 'account_group'),
      false
    )
  })
})
