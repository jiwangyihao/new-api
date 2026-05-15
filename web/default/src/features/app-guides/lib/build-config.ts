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

type ImportConfig = {
  id: string
  baseUrl: string
  apiKey: string
}

const OPENAI_BASE_PATH = '/v1'

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

export function buildCherryStudioConfig(
  serverAddress: string,
  apiKey: string
): ImportConfig {
  return {
    id: 'new-api',
    baseUrl: buildOpenAIBaseUrl(serverAddress),
    apiKey: normalizeApiKey(apiKey),
  }
}

export function encodeImportConfig(config: ImportConfig): string {
  const value = JSON.stringify(config)
  if (typeof window !== 'undefined' && typeof window.btoa === 'function') {
    return encodeURIComponent(window.btoa(value))
  }

  type BufferConstructorLike = {
    from(data: string, encoding: string): { toString(encoding: string): string }
  }
  const bufferCtor = (globalThis as Record<string, unknown>).Buffer
  if (
    typeof bufferCtor === 'function' &&
    typeof (bufferCtor as unknown as BufferConstructorLike).from === 'function'
  ) {
    return encodeURIComponent(
      (bufferCtor as unknown as BufferConstructorLike)
        .from(value, 'utf-8')
        .toString('base64')
    )
  }

  return encodeURIComponent(value)
}

export function buildOpenAICompatibleConfig(
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

export function buildOpenAICompatibleEnv(
  serverAddress: string,
  apiKey: string
): string {
  return [
    `OPENAI_BASE_URL=${buildOpenAIBaseUrl(serverAddress)}`,
    `OPENAI_API_KEY=${normalizeApiKey(apiKey)}`,
  ].join('\n')
}

export function buildCCSwitchImportUrl(params: {
  app: string
  name: string
  serverAddress: string
  apiKey: string
  model?: string
}): string {
  const trimmedServerAddress = params.serverAddress.trim().replace(/\/+$/, '')
  const search = new URLSearchParams()
  search.set('resource', 'provider')
  search.set('app', params.app)
  search.set('name', params.name)
  search.set(
    'endpoint',
    params.app === 'codex'
      ? buildOpenAIBaseUrl(trimmedServerAddress)
      : trimmedServerAddress
  )
  search.set('apiKey', normalizeApiKey(params.apiKey))
  if (params.model) search.set('model', params.model)
  search.set('homepage', trimmedServerAddress)
  search.set('enabled', 'true')
  return `ccswitch://v1/import?${search.toString()}`
}

export function buildQueryImportUrl(
  scheme: string,
  serverAddress: string,
  apiKey: string
): string {
  const search = new URLSearchParams()
  search.set('base_url', buildOpenAIBaseUrl(serverAddress))
  search.set('api_key', normalizeApiKey(apiKey))
  return `${scheme}?${search.toString()}`
}

export function buildContinueConfig(
  serverAddress: string,
  apiKey: string,
  model = 'gpt-4o-mini'
): string {
  return JSON.stringify(
    {
      models: [
        {
          title: 'new-api',
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
