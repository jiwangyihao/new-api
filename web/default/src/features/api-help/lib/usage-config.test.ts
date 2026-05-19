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
  buildAgentConfigArtifactPath,
  buildAgentConfigArtifactQueryKey,
  buildAgentConfigGuideInstruction,
  buildAgentConfigGuidePath,
  buildAgentConfigGuideURL,
  buildAgentConfigSections,
  buildCherryStudioConfig,
  buildContinueConfig,
  buildGenericEnvConfig,
  buildGenericJsonConfig,
  buildOpenAIBaseUrl,
  buildOpenCodeMetadataQueryKey,
  canFetchAgentConfigArtifacts,
  normalizeApiKey,
} from './usage-config'

const usageConfigSource = readFileSync(new URL('./usage-config.ts', import.meta.url), 'utf8')
const keysApiSource = readFileSync(new URL('../../keys/api.ts', import.meta.url), 'utf8')

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

  test('builds generic and common app manual snippets', () => {
    assert.equal(
      buildGenericEnvConfig('https://api.example.com/', 'live'),
      'OPENAI_BASE_URL=https://api.example.com/v1\nOPENAI_API_KEY=sk-live'
    )
    assert.deepEqual(
      JSON.parse(buildGenericJsonConfig('https://api.example.com', 'live')),
      {
        base_url: 'https://api.example.com/v1',
        api_key: 'sk-live',
      }
    )
    assert.deepEqual(
      JSON.parse(buildCherryStudioConfig('https://api.example.com/v1', 'sk-live')),
      {
        id: 'new-api',
        baseUrl: 'https://api.example.com/v1',
        apiKey: 'sk-live',
      }
    )
    assert.deepEqual(
      JSON.parse(buildContinueConfig('https://api.example.com/', 'live', 'gpt-5')),
      {
        models: [
          {
            title: 'new-api',
            provider: 'openai',
            model: 'gpt-5',
            apiBase: 'https://api.example.com/v1',
            apiKey: 'sk-live',
          },
        ],
      }
    )
  })

  test('builds AI agent auto-configuration manifest links safely', () => {
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

  test('builds metadata and artifact query keys with selected key and normalized artifact identity', () => {
    assert.deepEqual(
      buildOpenCodeMetadataQueryKey(42),
      ['api-help', 'opencode-openai-models', 'token', '42']
    )

    assert.deepEqual(
      buildAgentConfigArtifactQueryKey({
        client: 'opencode',
        file: 'opencode.json',
        selectedKeyId: 42,
        apiKey: 'sk-live',
        serverAddress: 'https://api.example.com',
      }),
      [
        'api-help',
        'agent-config-artifact',
        'opencode',
        'opencode.json',
        'token',
        '42',
        '/config-guides/opencode-openai/opencode.json',
        'https://api.example.com/v1',
      ]
    )

    assert.deepEqual(
      buildAgentConfigArtifactQueryKey({
        client: 'omp',
        file: 'models.yml',
        selectedKeyId: '7',
        apiKey: 'live',
        serverAddress: 'https://api.example.com/v1/',
      }),
      [
        'api-help',
        'agent-config-artifact',
        'omp',
        'models.yml',
        'token',
        '7',
        '/config-guides/omp-openai/models.yml',
        'https://api.example.com/v1',
      ]
    )
  })

  test('builds concrete artifact paths with normalized API key and base URL', () => {
    assert.equal(
      buildAgentConfigArtifactPath('opencode', 'opencode.json', 'sk-live', 'https://api.example.com'),
      '/config-guides/opencode-openai/opencode.json?api_key=sk-live&base_url=https%3A%2F%2Fapi.example.com%2Fv1'
    )
    assert.equal(
      buildAgentConfigArtifactPath('omp', 'models.yml', 'live', 'https://api.example.com/v1'),
      '/config-guides/omp-openai/models.yml?api_key=sk-live&base_url=https%3A%2F%2Fapi.example.com%2Fv1'
    )
    assert.equal(
      buildAgentConfigArtifactPath('omp', 'config.yml', 'sk-live', ''),
      '/config-guides/omp-openai/config.yml?api_key=sk-live'
    )
  })

  test('gates artifact fetching on selected key, ready metadata, and unmasked API key', () => {
    assert.equal(
      canFetchAgentConfigArtifacts({ selectedKeyId: '42', metadataReady: true, apiKey: 'sk-live' }),
      true
    )
    assert.equal(
      canFetchAgentConfigArtifacts({ selectedKeyId: '', metadataReady: true, apiKey: 'sk-live' }),
      false
    )
    assert.equal(
      canFetchAgentConfigArtifacts({ selectedKeyId: '42', metadataReady: false, apiKey: 'sk-live' }),
      false
    )
    assert.equal(
      canFetchAgentConfigArtifacts({ selectedKeyId: '42', metadataReady: true, apiKey: '' }),
      false
    )
  })

  test('builds backend artifact config sections only when ready', () => {
    assert.deepEqual(
      buildAgentConfigSections({ client: 'opencode', ready: false, artifacts: { 'opencode.json': '{}' } }),
      []
    )
    assert.deepEqual(
      buildAgentConfigSections({ client: 'opencode', ready: true, artifacts: { 'opencode.json': '{}' } }),
      [
        {
          path: 'opencode.json',
          content: '{}',
        },
      ]
    )
    assert.deepEqual(
      buildAgentConfigSections({
        client: 'omp',
        ready: true,
        artifacts: {
          'models.yml': 'models',
          'config.yml': 'config',
        },
      }),
      [
        {
          path: '~/.omp/agent/models.yml',
          content: 'models',
        },
        {
          path: '~/.omp/agent/config.yml',
          content: 'config',
        },
      ]
    )
    assert.equal(
      buildAgentConfigSections({
        client: 'omp',
        ready: true,
        artifacts: {
          'models.yml': 'models',
          'config.yml': 'config',
        },
      }).some((file) => file.path.includes('plugin') || file.path.includes('image-generator')),
      false
    )
  })

  test('usage config does not contain provider-native OMP image helpers', () => {
    for (const forbidden of [
      'IMAGE_PROVIDER_ID',
      'OMP_PROVIDER_TOOLS_PACKAGE',
      'buildOmpPluginInstructions',
      'buildOmpImageGeneratorConfig',
      'openaiProviderTools',
      'imageGeneration',
      'new-api-image',
    ]) {
      assert.doesNotMatch(usageConfigSource, new RegExp(forbidden))
    }
  })

  test('keys API requests OpenCode metadata by token id rather than API key', () => {
    assert.match(keysApiSource, /getOpenCodeOpenAIModels\(tokenId: number\)/)
    assert.match(keysApiSource, /params\.set\('token_id', String\(tokenId\)\)/)
    assert.doesNotMatch(keysApiSource, /api_key/)
  })
})
