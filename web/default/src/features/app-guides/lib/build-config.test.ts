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
import {
  buildCherryStudioConfig,
  buildContinueConfig,
  buildOpenAIBaseUrl,
  buildOpenAICompatibleConfig,
  buildOpenAICompatibleEnv,
  buildCCSwitchImportUrl,
  buildQueryImportUrl,
} from './build-config'

describe('app guide config builders', () => {
  test('normalizes the OpenAI-compatible base URL to one /v1 suffix', () => {
    assert.equal(buildOpenAIBaseUrl('https://api.example.com'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/v1'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/v1/'), 'https://api.example.com/v1')
  })

  test('builds Cherry Studio import payload with normalized key and base URL', () => {
    assert.deepEqual(buildCherryStudioConfig('https://api.example.com/', 'abc123'), {
      id: 'new-api',
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-abc123',
    })
  })

  test('does not double-prefix already normalized API keys', () => {
    assert.deepEqual(buildCherryStudioConfig('https://api.example.com', 'sk-live'), {
      id: 'new-api',
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-live',
    })
  })

  test('builds OpenAI-compatible JSON and environment snippets', () => {
    assert.equal(
      buildOpenAICompatibleConfig('https://api.example.com', 'sk-live'),
      '{\n  "base_url": "https://api.example.com/v1",\n  "api_key": "sk-live"\n}'
    )
    assert.equal(
      buildOpenAICompatibleEnv('https://api.example.com/', 'live'),
      'OPENAI_BASE_URL=https://api.example.com/v1\nOPENAI_API_KEY=sk-live'
    )
    assert.equal(
      buildContinueConfig('https://api.example.com/', 'live'),
      '{\n  "models": [\n    {\n      "title": "new-api",\n      "provider": "openai",\n      "model": "gpt-4o-mini",\n      "apiBase": "https://api.example.com/v1",\n      "apiKey": "sk-live"\n    }\n  ]\n}'
    )
  })

  test('builds protocol import URLs', () => {
    assert.equal(
      buildQueryImportUrl('chatbox://import', 'https://api.example.com', 'live'),
      'chatbox://import?base_url=https%3A%2F%2Fapi.example.com%2Fv1&api_key=sk-live'
    )
    assert.equal(
      buildCCSwitchImportUrl({
        app: 'claude',
        name: 'new-api',
        serverAddress: 'https://api.example.com/',
        apiKey: 'live',
        model: 'gpt-4o-mini',
      }),
      'ccswitch://v1/import?resource=provider&app=claude&name=new-api&endpoint=https%3A%2F%2Fapi.example.com&apiKey=sk-live&model=gpt-4o-mini&homepage=https%3A%2F%2Fapi.example.com&enabled=true'
    )
  })
})
