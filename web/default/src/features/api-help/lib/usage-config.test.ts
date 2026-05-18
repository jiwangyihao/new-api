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
  buildGenericEnvConfig,
  buildOmpModelsConfig,
  buildOmpSettingsConfig,
  buildOpenAIBaseUrl,
  buildOpenCodeConfig,
  normalizeApiKey,
} from './usage-config'

describe('api usage help config builders', () => {
  test('normalizes OpenAI-compatible base URLs and API keys', () => {
    assert.equal(buildOpenAIBaseUrl('https://api.example.com'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/v1'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/v1/'), 'https://api.example.com/v1')
    assert.equal(normalizeApiKey('live'), 'sk-live')
    assert.equal(normalizeApiKey('sk-live'), 'sk-live')
  })

  test('builds OpenCode config for the selected OpenAI-compatible model', () => {
    const config = JSON.parse(
      buildOpenCodeConfig('https://api.example.com/', 'live', 'gpt-4o-mini')
    )

    assert.equal(config.$schema, 'https://opencode.ai/config.json')
    assert.equal(config.model, 'new-api/gpt-4o-mini')
    assert.equal(config.small_model, 'new-api/gpt-4o-mini')
    assert.equal(config.provider['new-api'].npm, '@ai-sdk/openai-compatible')
    assert.equal(config.provider['new-api'].name, 'new-api')
    assert.equal(config.provider['new-api'].options.baseURL, 'https://api.example.com/v1')
    assert.equal(config.provider['new-api'].options.apiKey, 'sk-live')
    assert.equal(config.provider['new-api'].models['gpt-4o-mini'].name, 'gpt-4o-mini')
  })

  test('builds OMP models.yml with explicit zero cost fields', () => {
    const models = buildOmpModelsConfig(
      'https://api.example.com',
      'live',
      'gpt-4o-mini'
    )

    assert.match(models, /providers:\n  new-api:/)
    assert.match(models, /api: openai-responses/)
    assert.match(models, /baseUrl: https:\/\/api\.example\.com\/v1/)
    assert.match(models, /apiKey: sk-live/)
    assert.match(models, /id: gpt-4o-mini/)
    assert.match(models, /cacheRead: 0/)
    assert.match(models, /cacheWrite: 0/)
  })

  test('builds OMP role settings and generic env snippets', () => {
    assert.equal(
      buildOmpSettingsConfig('gpt-4o-mini'),
      'modelRoles:\n  default: new-api/gpt-4o-mini\n  slow: new-api/gpt-4o-mini\n  smol: new-api/gpt-4o-mini\n  plan: new-api/gpt-4o-mini\n  task: new-api/gpt-4o-mini\n  vision: new-api/gpt-4o-mini\n  designer: new-api/gpt-4o-mini\n  commit: new-api/gpt-4o-mini'
    )
    assert.equal(
      buildGenericEnvConfig('https://api.example.com/', 'live'),
      'OPENAI_BASE_URL=https://api.example.com/v1\nOPENAI_API_KEY=sk-live'
    )
  })
})
