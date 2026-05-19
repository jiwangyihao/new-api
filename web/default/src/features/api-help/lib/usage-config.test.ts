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
  buildAgentConfigGuideInstruction,
  buildAgentConfigGuidePath,
  buildAgentConfigGuideURL,
  buildGenericEnvConfig,
  buildOmpModelsConfig,
  buildOmpSettingsConfig,
  buildOpenAIBaseUrl,
  buildOpenCodeConfig,
  normalizeApiKey,
  type OpenCodeOpenAIModel,
} from './usage-config'

describe('api usage help config builders', () => {
  test('normalizes OpenAI-compatible base URLs and API keys', () => {
    assert.equal(buildOpenAIBaseUrl('https://api.example.com'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/v1'), 'https://api.example.com/v1')
    assert.equal(buildOpenAIBaseUrl('https://api.example.com/v1/'), 'https://api.example.com/v1')
    assert.equal(normalizeApiKey('live'), 'sk-live')
    assert.equal(normalizeApiKey('sk-live'), 'sk-live')
    assert.equal(normalizeApiKey('sk-sk-live'), 'sk-live')
  })

  test('builds full OpenCode config from OpenAI-compatible metadata', () => {
    const metadata: Record<string, OpenCodeOpenAIModel> = {
      'gpt-5': {
        id: 'gpt-5',
        name: 'GPT-5',
        attachment: true,
        reasoning: true,
        tool_call: true,
        structured_output: true,
        temperature: false,
        modalities: { input: ['text', 'image'], output: ['text'] },
        cost: { input: 5, output: 30, cache_read: 0.5, cache_write: 0 },
        limit: { context: 272000, output: 128000 },
        release_date: '2026-01-01',
      },
      'gpt-5-fast': {
        id: 'gpt-5-fast',
        name: 'GPT-5 Fast',
        attachment: true,
        reasoning: true,
        tool_call: true,
        structured_output: true,
        temperature: false,
        modalities: { input: ['text', 'image'], output: ['text'] },
        cost: { input: 5, output: 30 },
        limit: { context: 272000, output: 128000 },
        headers: { 'x-test': 'fast' },
      },
      'gpt-5-mini': {
        id: 'gpt-5-mini',
        name: 'GPT-5 mini',
        attachment: true,
        reasoning: true,
        tool_call: true,
        structured_output: true,
        temperature: false,
        modalities: { input: ['text'], output: ['text'] },
        cost: { input: 0.75, output: 4.5, cache_read: 0.075, cache_write: 0 },
        limit: { context: 272000, output: 128000 },
      },
      'gpt-5-text-incomplete': {
        id: 'gpt-5-text-incomplete',
        name: 'GPT-5 Text Incomplete',
        attachment: true,
        reasoning: true,
        tool_call: true,
        structured_output: true,
        temperature: false,
        modalities: { input: ['text'], output: ['text'] },
        cost: { input: 1 },
        limit: { input: 128000 },
      },
      'gpt-5-image-only': {
        id: 'gpt-5-image-only',
        name: 'GPT-5 Image Only',
        attachment: true,
        reasoning: false,
        tool_call: false,
        structured_output: false,
        temperature: false,
        modalities: { input: ['text'], output: ['image'] },
        cost: { input: 1 },
        limit: { input: 128000 },
      },
    }
    const config = JSON.parse(
      buildOpenCodeConfig('https://api.example.com/', 'live', 'gpt-5', metadata)
    )

    assert.equal(config.$schema, 'https://opencode.ai/config.json')
    assert.equal(config.provider['new-api'].npm, '@ai-sdk/openai')
    assert.equal(config.provider['new-api'].options.baseURL, 'https://api.example.com/v1')
    assert.equal(config.provider['new-api'].options.apiKey, 'sk-live')
    assert.equal(config.provider['new-api'].models['gpt-5'].cost.cache_read, 0.5)
    assert.equal(config.provider['new-api'].models['gpt-5'].options.store, false)
    assert.equal(config.provider['new-api'].models['gpt-5'].options.metadata, undefined)
    assert.equal(config.provider['new-api'].models['gpt-5-fast'].id, 'gpt-5')
    assert.equal(config.provider['new-api'].models['gpt-5-fast'].headers['x-test'], 'fast')
    assert.equal(Object.keys(config.provider['new-api'].models).some((id) => id.endsWith('-Sys')), false)
    assert.equal(config.agent.image, undefined)
    assert.equal(config.provider['new-api'].models['gpt-5'].variants.image, undefined)
    assert.equal(config.provider['new-api'].models['gpt-5-fast'].structured_output, undefined)
    assert.equal(config.model, 'new-api/gpt-5')
    assert.equal(config.small_model, 'new-api/gpt-5-mini')
    assert.equal(config.provider['new-api'].models['gpt-5-text-incomplete'], undefined)
    assert.equal(config.provider['new-api'].models['gpt-5-image-only'], undefined)
  })

  test('builds OMP models.yml with provider tools and full model metadata', () => {
    const metadata: Record<string, OpenCodeOpenAIModel> = {
      'gpt-5': {
        id: 'gpt-5',
        name: 'OpenAI: GPT-5',
        attachment: true,
        reasoning: true,
        tool_call: true,
        structured_output: true,
        temperature: false,
        modalities: { input: ['text', 'image'], output: ['text'] },
        cost: { input: 5, output: 30, cache_read: 0.5, cache_write: 0 },
        limit: { context: 272000, output: 128000 },
      },
      'gpt-5-mini': {
        id: 'gpt-5-mini',
        name: 'OpenAI: GPT-5 mini',
        attachment: true,
        reasoning: true,
        tool_call: true,
        structured_output: true,
        temperature: false,
        modalities: { input: ['text'], output: ['text'] },
        cost: { input: 0.75, output: 4.5, cache_read: 0.075, cache_write: 0 },
        limit: { context: 272000, output: 128000 },
      },
    }
    const models = buildOmpModelsConfig(
      'https://api.example.com',
      'live',
      'gpt-5',
      metadata,
      '9.9.9'
    )

    assert.match(models, /omp plugin install npm:omp-openai-provider-tools@9\.9\.9/)
    assert.match(models, /providers:\n  new-api:/)
    assert.match(models, /compat:\n      openaiProviderTools:\n        enabled: true/)
    assert.match(models, /baseUrl: https:\/\/api\.example\.com\/v1/)
    assert.match(models, /apiKey: sk-live/)
    assert.match(models, /id: gpt-5\n        name: "OpenAI: GPT-5"/)
    assert.match(models, /contextWindow: 272000/)
    assert.match(models, /cacheRead: 0\.5/)
    assert.match(models, /new-api\/gpt-5: gpt-5/)
    assert.match(models, /new-api-image:\n    api: openai-responses/)
    assert.match(models, /imageGeneration: true/)
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

  test('builds AI agent auto-configuration guide links safely', () => {
    assert.equal(
      buildAgentConfigGuidePath('omp', 'sk-live', 'https://api.example.com/v1'),
      '/config-guides/omp-openai/manifest.json?api_key=sk-live&base_url=https%3A%2F%2Fapi.example.com%2Fv1'
    )
    assert.equal(
      buildAgentConfigGuideURL('https://api.example.com/v1', 'opencode', 'sk-live'),
      'https://api.example.com/config-guides/opencode-openai/manifest.json?api_key=sk-live&base_url=https%3A%2F%2Fapi.example.com%2Fv1'
    )
    assert.equal(
      buildAgentConfigGuideInstruction('https://api.example.com/v1', 'omp', 'sk-live'),
      'Use this manifest to auto-configure OMP: https://api.example.com/config-guides/omp-openai/manifest.json?api_key=sk-live&base_url=https%3A%2F%2Fapi.example.com%2Fv1'
    )
  })
})
