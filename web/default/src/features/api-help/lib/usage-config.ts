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

const OPENAI_BASE_PATH = '/v1'
const PROVIDER_ID = 'new-api'

export function normalizeApiKey(apiKey: string): string {
  const trimmed = apiKey.trim()
  if (!trimmed) return ''
  return trimmed.startsWith('sk-') ? trimmed : `sk-${trimmed}`
}

export function buildOpenAIBaseUrl(serverAddress: string): string {
  const trimmed = serverAddress.trim().replace(/\/+$/, '')
  if (!trimmed) return OPENAI_BASE_PATH
  if (trimmed.toLowerCase().endsWith(OPENAI_BASE_PATH)) return trimmed
  return `${trimmed}${OPENAI_BASE_PATH}`
}

export function buildGenericJsonConfig(
  serverAddress: string,
  apiKey: string
): string {
  return JSON.stringify(
    {
      base_url: buildOpenAIBaseUrl(serverAddress),
      api_key: normalizeApiKey(apiKey),
    },
    null,
    2
  )
}

export function buildGenericEnvConfig(
  serverAddress: string,
  apiKey: string
): string {
  return [
    `OPENAI_BASE_URL=${buildOpenAIBaseUrl(serverAddress)}`,
    `OPENAI_API_KEY=${normalizeApiKey(apiKey)}`,
  ].join('\n')
}

export function buildCherryStudioConfig(
  serverAddress: string,
  apiKey: string
): string {
  return JSON.stringify(
    {
      id: PROVIDER_ID,
      baseUrl: buildOpenAIBaseUrl(serverAddress),
      apiKey: normalizeApiKey(apiKey),
    },
    null,
    2
  )
}

export function buildContinueConfig(
  serverAddress: string,
  apiKey: string,
  model: string
): string {
  return JSON.stringify(
    {
      models: [
        {
          title: PROVIDER_ID,
          provider: 'openai',
          model,
          apiBase: buildOpenAIBaseUrl(serverAddress),
          apiKey: normalizeApiKey(apiKey),
        },
      ],
    },
    null,
    2
  )
}

export function buildOpenCodeConfig(
  serverAddress: string,
  apiKey: string,
  model: string
): string {
  return JSON.stringify(
    {
      $schema: 'https://opencode.ai/config.json',
      model: `${PROVIDER_ID}/${model}`,
      small_model: `${PROVIDER_ID}/${model}`,
      provider: {
        [PROVIDER_ID]: {
          npm: '@ai-sdk/openai-compatible',
          name: PROVIDER_ID,
          options: {
            baseURL: buildOpenAIBaseUrl(serverAddress),
            apiKey: normalizeApiKey(apiKey),
          },
          models: {
            [model]: {
              name: model,
            },
          },
        },
      },
    },
    null,
    2
  )
}

export function buildOmpModelsConfig(
  serverAddress: string,
  apiKey: string,
  model: string
): string {
  return `providers:
  ${PROVIDER_ID}:
    api: openai-responses
    baseUrl: ${buildOpenAIBaseUrl(serverAddress)}
    apiKey: ${normalizeApiKey(apiKey)}
    models:
      - id: ${model}
        name: ${model}
        api: openai-responses
        reasoning: true
        input:
          - text
          - image
        cost:
          input: 0
          output: 0
          cacheRead: 0
          cacheWrite: 0

equivalence:
  overrides:
    ${PROVIDER_ID}/${model}: ${model}`
}

export function buildOmpSettingsConfig(model: string): string {
  const selector = `${PROVIDER_ID}/${model}`
  return [
    'modelRoles:',
    `  default: ${selector}`,
    `  slow: ${selector}`,
    `  smol: ${selector}`,
    `  plan: ${selector}`,
    `  task: ${selector}`,
    `  vision: ${selector}`,
    `  designer: ${selector}`,
    `  commit: ${selector}`,
  ].join('\n')
}
