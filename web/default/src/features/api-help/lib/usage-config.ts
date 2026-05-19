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

export type OpenCodeOpenAIModel = {
  id: string
  name: string
  family?: string
  attachment: boolean
  reasoning: boolean
  tool_call: boolean
  structured_output: boolean
  temperature: boolean
  knowledge?: string
  interleaved?: unknown
  modalities?: {
    input?: string[]
    output?: string[]
  }
  cost?: {
    input?: number
    output?: number
    cache_read?: number
    cache_write?: number
    context_over_200k?: {
      input?: number
      output?: number
      cache_read?: number
      cache_write?: number
    }
  }
  limit?: {
    context?: number
    input?: number
    output?: number
  }
  release_date?: string
  options?: Record<string, unknown>
  headers?: Record<string, string>
}

export type OpenCodeOpenAIModelsResponse = {
  models: Record<string, OpenCodeOpenAIModel>
}

export type AgentConfigGuideClient = 'omp' | 'opencode'
export type AgentConfigArtifactFile = 'opencode.json' | 'models.yml' | 'config.yml'
export type ConfigFile = { path: string; content: string; hint?: string }

export type AgentConfigArtifactQueryInput = {
  client: AgentConfigGuideClient
  file: AgentConfigArtifactFile
  selectedKeyId?: string | number
  apiKey: string
  serverAddress: string
}

export function normalizeApiKey(apiKey: string): string {
  let trimmed = apiKey.trim()
  while (trimmed.startsWith('sk-sk-')) trimmed = trimmed.slice(3)
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

export function buildAgentConfigGuidePath(
  client: AgentConfigGuideClient,
  apiKey: string,
  baseUrl?: string
): string {
  const params = new URLSearchParams()
  params.set('api_key', normalizeApiKey(apiKey))
  const normalizedBaseUrl = normalizeOptionalBaseUrl(baseUrl ?? '')
  if (normalizedBaseUrl) params.set('base_url', normalizedBaseUrl)
  const query = params.toString()
  return `${getAgentConfigGuideBasePath(client)}/manifest.json${query ? `?${query}` : ''}`
}

export function getAgentConfigGuideOrigin(serverAddress: string): string {
  const trimmed = serverAddress.trim()
  if (!trimmed) return ''

  try {
    const url = new URL(trimmed)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return ''
    if (url.username || url.password || url.search || url.hash) return ''
    url.pathname = url.pathname.replace(/\/+$/, '').replace(/\/v1$/i, '')
    return url.toString().replace(/\/+$/, '')
  } catch {
    return ''
  }
}

export function buildAgentConfigGuideURL(
  serverAddress: string,
  client: AgentConfigGuideClient,
  apiKey: string
): string {
  const origin = getAgentConfigGuideOrigin(serverAddress)
  const path = buildAgentConfigGuidePath(
    client,
    apiKey,
    origin ? buildOpenAIBaseUrl(origin) : undefined
  )
  return origin ? `${origin}${path}` : path
}

export function buildAgentConfigGuideInstruction(
  serverAddress: string,
  client: AgentConfigGuideClient,
  apiKey: string
): string {
  const label = client === 'omp' ? 'OMP' : 'OpenCode'
  return `Use this manifest to auto-configure ${label}: ${buildAgentConfigGuideURL(
    serverAddress,
    client,
    apiKey
  )}`
}

export function buildOpenCodeMetadataQueryKey(
  selectedKeyId: string | number | undefined
): readonly unknown[] {
  return [
    'api-help',
    'opencode-openai-models',
    'token',
    String(selectedKeyId ?? ''),
  ] as const
}

export function buildAgentConfigArtifactPath(
  client: AgentConfigGuideClient,
  file: AgentConfigArtifactFile,
  apiKey: string,
  serverAddress: string
): string {
  const params = new URLSearchParams()
  params.set('api_key', normalizeApiKey(apiKey))
  const normalizedBaseUrl = normalizeOptionalBaseUrl(serverAddress)
  if (normalizedBaseUrl) params.set('base_url', normalizedBaseUrl)
  return `${getAgentConfigArtifactRoutePath(client, file)}?${params.toString()}`
}

export function buildAgentConfigArtifactQueryKey(
  input: AgentConfigArtifactQueryInput
): readonly unknown[] {
  return [
    'api-help',
    'agent-config-artifact',
    input.client,
    input.file,
    'token',
    String(input.selectedKeyId ?? ''),
    getAgentConfigArtifactRoutePath(input.client, input.file),
    normalizeOptionalBaseUrl(input.serverAddress),
  ] as const
}

export function canFetchAgentConfigArtifacts(input: {
  selectedKeyId?: string | number
  metadataReady: boolean
  apiKey: string
}): boolean {
  return Boolean(String(input.selectedKeyId ?? '').trim()) &&
    input.metadataReady &&
    normalizeApiKey(input.apiKey).length > 'sk-'.length
}

export function buildAgentConfigSections(input: {
  client: AgentConfigGuideClient
  ready: boolean
  artifacts: Partial<Record<AgentConfigArtifactFile, string>>
}): ConfigFile[] {
  if (!input.ready) return []

  if (input.client === 'opencode') {
    return [
      {
        path: 'opencode.json',
        content: input.artifacts['opencode.json'] ?? '',
      },
    ]
  }

  return [
    {
      path: '~/.omp/agent/models.yml',
      content: input.artifacts['models.yml'] ?? '',
    },
    {
      path: '~/.omp/agent/config.yml',
      content: input.artifacts['config.yml'] ?? '',
    },
  ]
}

function normalizeOptionalBaseUrl(serverAddress: string): string {
  const trimmed = serverAddress.trim()
  return trimmed ? buildOpenAIBaseUrl(trimmed) : ''
}

function getAgentConfigGuideBasePath(client: AgentConfigGuideClient): string {
  return `/config-guides/${client === 'omp' ? 'omp-openai' : 'opencode-openai'}`
}

function getAgentConfigArtifactRoutePath(
  client: AgentConfigGuideClient,
  file: AgentConfigArtifactFile
): string {
  return `${getAgentConfigGuideBasePath(client)}/${file}`
}
