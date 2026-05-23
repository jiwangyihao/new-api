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
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from './channel-form'
import type { Channel } from '../types'

const channelDrawerSource = readFileSync(
  new URL('../components/drawers/channel-mutate-drawer.tsx', import.meta.url),
  'utf8'
)

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
        supported_endpoint_types: ['openai-response', 'openai-response-compact'],
      }),
    } as unknown as Channel

    const defaults = transformChannelToFormDefaults(channel)

    assert.deepEqual(defaults.supported_endpoint_types, [
      'openai-response',
      'openai-response-compact',
    ])
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

    assert.deepEqual(JSON.parse(payload.channel.settings || '{}'), {
      supported_endpoint_types: ['openai-response', 'openai-response-compact'],
      allow_service_tier: false,
      disable_store: false,
      allow_safety_identifier: false,
      allow_include_obfuscation: false,
      allow_inference_geo: false,
      upstream_model_update_check_enabled: false,
      upstream_model_update_auto_sync_enabled: false,
      upstream_model_update_ignored_models: [],
      upstream_model_update_last_check_time: 0,
      upstream_model_update_last_detected_models: [],
    })
  })

  test('exposes compact endpoint capability in channel drawer', () => {
    assert.match(channelDrawerSource, /CHANNEL_ENDPOINT_OPTIONS/)
    assert.match(channelDrawerSource, /openai-response-compact/)
    assert.match(channelDrawerSource, /name='supported_endpoint_types'/)
    assert.match(channelDrawerSource, /Endpoint Capabilities/)
    assert.match(channelDrawerSource, /Supported Endpoint Types/)
    assert.match(channelDrawerSource, /Use channel type defaults/)
  })
})
