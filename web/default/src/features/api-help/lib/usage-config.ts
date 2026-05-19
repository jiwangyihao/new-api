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
const IMAGE_PROVIDER_ID = 'new-api-image'
const OMP_PROVIDER_TOOLS_PACKAGE = 'omp-openai-provider-tools'
const DEFAULT_AGENT_MODEL_ID = 'gpt-5'
const SMALL_AGENT_MODEL_ID = 'gpt-5-mini'

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

export type OMPProviderToolsMetadata = {
  package: string
  latest_version: string
  status: 'ok' | 'cached' | 'unavailable'
  error?: string
}

export type OpenCodeOpenAIModelsResponse = {
  models: Record<string, OpenCodeOpenAIModel>
  omp_openai_provider_tools?: OMPProviderToolsMetadata
}

export type AgentConfigGuideClient = 'omp' | 'opencode'

type OMPModelConfig = {
  id: string
  name: string
  api: 'openai-responses'
  reasoning: boolean
  input: Array<'text' | 'image'>
  contextWindow?: number
  maxTokens?: number
  cost: {
    input: number
    output: number
    cacheRead: number
    cacheWrite: number
  }
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
  const clientPath = client === 'omp' ? 'omp-openai' : 'opencode-openai'
  const params = new URLSearchParams()
  params.set('api_key', normalizeApiKey(apiKey))
  const trimmedBaseUrl = baseUrl?.trim()
  if (trimmedBaseUrl) params.set('base_url', buildOpenAIBaseUrl(trimmedBaseUrl))
  const query = params.toString()
  return `/config-guides/${clientPath}/manifest.json${query ? `?${query}` : ''}`
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
  const path = buildAgentConfigGuidePath(client, apiKey, origin ? buildOpenAIBaseUrl(origin) : undefined)
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

export function buildOpenCodeConfig(
  serverAddress: string,
  apiKey: string,
  model: string,
  models?: Record<string, OpenCodeOpenAIModel>
): string {
  if (!models || Object.keys(models).length === 0) {
    return JSON.stringify(
      {
        $schema: 'https://opencode.ai/config.json',
        model: `${PROVIDER_ID}/${model}`,
        small_model: `${PROVIDER_ID}/${model}`,
        provider: {
          [PROVIDER_ID]: {
            npm: '@ai-sdk/openai',
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

  const openCodeModels = buildOpenCodeBaseModels(models)
  const defaultModel = selectOpenCodeDefaultModel(model, openCodeModels)

  return JSON.stringify(
    {
      provider: {
        [PROVIDER_ID]: {
          npm: '@ai-sdk/openai',
          name: PROVIDER_ID,
          options: {
            baseURL: buildOpenAIBaseUrl(serverAddress),
            apiKey: normalizeApiKey(apiKey),
          },
          models: openCodeModels,
        },
      },
      model: `${PROVIDER_ID}/${defaultModel}`,
      small_model: `${PROVIDER_ID}/${selectOpenCodeSmallModel(openCodeModels, defaultModel)}`,
      agent: {
        build: {
          options: {
            store: false,
          },
        },
        plan: {
          options: {
            store: false,
          },
        },
      },
      $schema: 'https://opencode.ai/config.json',
    },
    null,
    2
  )
}

export function buildOmpPluginInstructions(pluginVersion: string): string {
  const version = pluginVersion.trim() || 'latest'
  return `# 1. Install or upgrade provider-native tools plugin
omp plugin install npm:${OMP_PROVIDER_TOOLS_PACKAGE}@${version}

# 2. Check plugin health
omp plugin doctor

# 3. Preview the recommended image subagent template
npx ${OMP_PROVIDER_TOOLS_PACKAGE} configure-image-agent --model ${IMAGE_PROVIDER_ID}/${DEFAULT_AGENT_MODEL_ID}-Sys --dry-run

# 4. After reviewing the preview, write ~/.omp/agent/agents/image-generator.md
npx ${OMP_PROVIDER_TOOLS_PACKAGE} configure-image-agent --model ${IMAGE_PROVIDER_ID}/${DEFAULT_AGENT_MODEL_ID}-Sys

# If image_generator already exists, the command refuses to overwrite it.
# Use --print to inspect and merge manually; use --force only when you intentionally replace it.
npx ${OMP_PROVIDER_TOOLS_PACKAGE} configure-image-agent --model ${IMAGE_PROVIDER_ID}/${DEFAULT_AGENT_MODEL_ID}-Sys --print`
}

export function buildOmpModelsConfig(
  serverAddress: string,
  apiKey: string,
  model: string,
  models?: Record<string, OpenCodeOpenAIModel>,
  pluginVersion = 'latest'
): string {
  if (!models || Object.keys(models).length === 0) {
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

  const normalizedModels = withOMPSysVariants(normalizeOMPModels(models))
  const selectedModelYaml = Object.keys(normalizedModels)
    .sort()
    .map((id) => renderOMPModelYaml(normalizedModels[id]))
    .join('\n')
  const imageSource = normalizedModels[`${DEFAULT_AGENT_MODEL_ID}-Sys`] ?? normalizedModels[model]
  const imageModel = renderOMPModelYaml(
    {
      ...imageSource,
      id: imageSource?.id ?? model,
      name: `${imageSource?.name ?? model} Image`,
    },
    '      ',
    [
      '        compat:',
      '          openaiProviderTools:',
      '            imageGeneration: true',
    ]
  )

  return `# Image generation and provider-native web_search require this plugin:
#   omp plugin install npm:${OMP_PROVIDER_TOOLS_PACKAGE}@${pluginVersion.trim() || 'latest'}
#   omp plugin doctor
# Recommended image subagent command:
#   npx ${OMP_PROVIDER_TOOLS_PACKAGE} configure-image-agent --model ${IMAGE_PROVIDER_ID}/${DEFAULT_AGENT_MODEL_ID}-Sys --dry-run
# Restart OMP after installing or upgrading the plugin.
providers:
  ${PROVIDER_ID}:
    api: openai-responses
    baseUrl: ${buildOpenAIBaseUrl(serverAddress)}
    apiKey: ${normalizeApiKey(apiKey)}
    compat:
      openaiProviderTools:
        enabled: true
    models:
${selectedModelYaml}

  ${IMAGE_PROVIDER_ID}:
    api: openai-responses
    baseUrl: ${buildOpenAIBaseUrl(serverAddress)}
    apiKey: ${normalizeApiKey(apiKey)}
    compat:
      openaiProviderTools:
        enabled: true
    models:
${imageModel}

equivalence:
  overrides:
${buildOMPEquivalenceOverrides(normalizedModels)}`
}

export function buildOmpSettingsConfig(
  model: string,
  models?: Record<string, OpenCodeOpenAIModel>
): string {
  if (!models || Object.keys(models).length === 0) {
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

  const normalizedModels = withOMPSysVariants(normalizeOMPModels(models))
  const defaultModel = normalizedModels[`${DEFAULT_AGENT_MODEL_ID}-Sys`]
    ? `${DEFAULT_AGENT_MODEL_ID}-Sys`
    : normalizedModels[`${model}-Sys`]
      ? `${model}-Sys`
      : model
  const smolModel = normalizedModels[`${SMALL_AGENT_MODEL_ID}-Sys`]
    ? `${SMALL_AGENT_MODEL_ID}-Sys`
    : defaultModel
  const defaultSelector = `${PROVIDER_ID}/${defaultModel}`
  const smolSelector = `${PROVIDER_ID}/${smolModel}`

  return `defaultThinkingLevel: xhigh
serviceTier: priority

modelRoles:
  default: ${defaultSelector}
  slow: ${defaultSelector}
  smol: ${smolSelector}
  plan: ${defaultSelector}
  task: ${defaultSelector}:xhigh
  vision: ${defaultSelector}
  designer: ${defaultSelector}:xhigh
  commit: ${defaultSelector}:xhigh

task:
  agentModelOverrides:
    explore: ${smolSelector}:xhigh
    librarian: ${smolSelector}:xhigh
    reviewer: ${defaultSelector}:xhigh
    plan: ${defaultSelector}:xhigh`
}

export function buildOmpImageGeneratorConfig(model = `${DEFAULT_AGENT_MODEL_ID}-Sys`): string {
  return `---
name: image_generator
description: Generate or iterate images only; do not handle ordinary code modification tasks.
model: ${IMAGE_PROVIDER_ID}/${model}:xhigh
---

You are a specialized image generation subagent.

Use the provider-native image generation capability to create or refine images when the user explicitly asks for visual output. Do not take over normal coding, refactoring, debugging, or documentation tasks. Return concise status and generated image references to the caller.`
}

function buildOpenCodeBaseModels(
  source: Record<string, OpenCodeOpenAIModel>
): Record<string, Record<string, unknown>> {
  return Object.fromEntries(
    Object.keys(source)
      .sort()
      .flatMap((id) => isOpenCodeTextModel(source[id]) ? [[id, normalizeOpenCodeModelConfig(id, source[id])]] : [])
  )
}

function isOpenCodeTextModel(model: OpenCodeOpenAIModel): boolean {
  return (model.modalities?.output ?? []).includes('text') && isOpenCodeRequiredPricingComplete(model)
}

function isOpenCodeRequiredPricingComplete(model: OpenCodeOpenAIModel): boolean {
  return (model.cost?.input ?? 0) > 0 &&
    (model.cost?.output ?? 0) > 0 &&
    (model.limit?.context ?? 0) > 0 &&
    (model.limit?.output ?? 0) > 0
}

function normalizeOpenCodeModelConfig(
  id: string,
  model: OpenCodeOpenAIModel
): Record<string, unknown> {
  const config: Record<string, unknown> = {
    id: id.endsWith('-fast') ? id.slice(0, -'-fast'.length) : id,
    name: model.name,
    attachment: model.attachment,
    reasoning: model.reasoning,
    tool_call: model.tool_call,
    temperature: model.temperature,
    options: mergeOpenCodeModelOptions(model.options),
    variants: buildOpenCodeReasoningVariants(reasoningLevels(id, model)),
  }
  if (model.family) config.family = model.family
  if (model.knowledge) config.knowledge = model.knowledge
  if (model.interleaved !== undefined) config.interleaved = model.interleaved
  if (model.modalities?.input?.length || model.modalities?.output?.length) {
    config.modalities = {
      ...(model.modalities.input?.length ? { input: model.modalities.input } : {}),
      ...(model.modalities.output?.length ? { output: model.modalities.output } : {}),
    }
  }
  const cost = openCodeCost(model.cost)
  if (Object.keys(cost).length > 0) config.cost = cost
  const limit = openCodeLimit(model.limit)
  if (Object.keys(limit).length > 0) config.limit = limit
  if (model.release_date) config.release_date = model.release_date
  if (model.headers && Object.keys(model.headers).length > 0) {
    config.headers = { ...model.headers }
  }
  return config
}


function mergeOpenCodeModelOptions(options?: Record<string, unknown>): Record<string, unknown> {
  const out = deepCloneRecord(options)
  out.store = false
  return out
}

function buildOpenCodeReasoningVariants(levels: string[]): Record<string, unknown> {
  return Object.fromEntries(
    levels.map((level) => [
      level,
      {
        reasoningEffort: level,
        reasoningSummary: 'auto',
        include: ['reasoning.encrypted_content'],
      },
    ])
  )
}

function reasoningLevels(id: string, model: OpenCodeOpenAIModel): string[] {
  if (!model.reasoning) return []
  const lower = id.toLowerCase()
  if (lower === 'gpt-5-pro') return []
  if (
    lower === 'gpt-5-codex' ||
    lower === 'gpt-5.1-codex' ||
    lower === 'gpt-5.1-codex-max' ||
    lower === 'gpt-5.1-codex-mini' ||
    lower === 'codex-mini-latest'
  ) {
    return ['low', 'medium', 'high']
  }
  if (
    lower === 'gpt-5.3-codex-spark' ||
    lower === 'gpt-5.3-codex' ||
    lower === 'gpt-5.2-codex'
  ) {
    return ['low', 'medium', 'high', 'xhigh']
  }
  const levels = lower.includes('gpt-5-') || lower === 'gpt-5'
    ? ['minimal', 'low', 'medium', 'high']
    : ['low', 'medium', 'high']
  if ((model.release_date ?? '') >= '2025-11-13') levels.unshift('none')
  if ((model.release_date ?? '') >= '2025-12-04') levels.push('xhigh')
  return levels
}

function openCodeCost(cost?: OpenCodeOpenAIModel['cost']): Record<string, unknown> {
  if (!cost || (cost.input ?? 0) <= 0 || (cost.output ?? 0) <= 0) return {}
  const contextOver200K = cost.context_over_200k &&
    (cost.context_over_200k.input ?? 0) > 0 &&
    (cost.context_over_200k.output ?? 0) > 0
    ? compactRecord({
        input: cost.context_over_200k.input,
        output: cost.context_over_200k.output,
        cache_read: cost.context_over_200k.cache_read,
        cache_write: cost.context_over_200k.cache_write,
      })
    : undefined
  return compactRecord({
    input: cost.input,
    output: cost.output,
    cache_read: cost.cache_read,
    cache_write: cost.cache_write,
    context_over_200k: contextOver200K,
  })
}

function openCodeLimit(limit?: OpenCodeOpenAIModel['limit']): Record<string, unknown> {
  if (!limit || (limit.context ?? 0) <= 0 || (limit.output ?? 0) <= 0) return {}
  return compactRecord({
    context: limit.context,
    input: limit.input,
    output: limit.output,
  })
}

function selectOpenCodeDefaultModel(
  preferred: string,
  models: Record<string, Record<string, unknown>>
): string {
  if (models[preferred]) return preferred
  if (models[DEFAULT_AGENT_MODEL_ID]) return DEFAULT_AGENT_MODEL_ID
  return Object.keys(models).sort()[0] ?? preferred
}

function selectOpenCodeSmallModel(
  models: Record<string, Record<string, unknown>>,
  fallback: string
): string {
  if (models[SMALL_AGENT_MODEL_ID]) return SMALL_AGENT_MODEL_ID
  return fallback
}


function normalizeOMPModels(
  source: Record<string, OpenCodeOpenAIModel>
): Record<string, OMPModelConfig> {
  return Object.fromEntries(
    Object.keys(source)
      .filter((id) => !id.endsWith('-fast'))
      .sort()
      .map((id) => [id, normalizeOMPModelConfig(source[id])])
  )
}

function normalizeOMPModelConfig(model: OpenCodeOpenAIModel): OMPModelConfig {
  const input = (model.modalities?.input ?? []).filter(
    (item): item is 'text' | 'image' => item === 'text' || item === 'image'
  )
  return {
    id: model.id,
    name: model.name,
    api: 'openai-responses',
    reasoning: model.reasoning,
    input: input.length > 0 ? input : ['text'],
    contextWindow: model.limit?.input && model.limit.input > 0
      ? model.limit.input
      : model.limit?.context,
    maxTokens: model.limit?.output,
    cost: {
      input: model.cost?.input ?? 0,
      output: model.cost?.output ?? 0,
      cacheRead: model.cost?.cache_read ?? 0,
      cacheWrite: model.cost?.cache_write ?? 0,
    },
  }
}

function withOMPSysVariants(
  models: Record<string, OMPModelConfig>
): Record<string, OMPModelConfig> {
  const expanded: Record<string, OMPModelConfig> = {}
  for (const id of Object.keys(models).sort()) {
    expanded[id] = cloneOMPModelConfig(models[id])
    expanded[`${id}-Sys`] = {
      ...cloneOMPModelConfig(models[id]),
      id: `${models[id].id}-Sys`,
      name: `${models[id].name} (Sys)`,
    }
  }
  return expanded
}

function cloneOMPModelConfig(model: OMPModelConfig): OMPModelConfig {
  return {
    ...model,
    input: [...model.input],
    cost: { ...model.cost },
  }
}

function renderOMPModelYaml(
  model: OMPModelConfig,
  indent = '      ',
  extraLines: string[] = []
): string {
  const lines = [
    `${indent}- id: ${model.id}`,
    `${indent}  name: ${yamlDoubleQuotedScalar(model.name)}`,
    `${indent}  api: ${model.api}`,
    `${indent}  reasoning: ${model.reasoning ? 'true' : 'false'}`,
    `${indent}  input:`,
    ...model.input.map((item) => `${indent}    - ${item}`),
  ]
  if (model.contextWindow !== undefined) {
    lines.push(`${indent}  contextWindow: ${model.contextWindow}`)
  }
  if (model.maxTokens !== undefined) {
    lines.push(`${indent}  maxTokens: ${model.maxTokens}`)
  }
  lines.push(`${indent}  cost:`)
  lines.push(`${indent}    input: ${formatNumber(model.cost.input)}`)
  lines.push(`${indent}    output: ${formatNumber(model.cost.output)}`)
  lines.push(`${indent}    cacheRead: ${formatNumber(model.cost.cacheRead)}`)
  lines.push(`${indent}    cacheWrite: ${formatNumber(model.cost.cacheWrite)}`)
  lines.push(...extraLines)
  return lines.join('\n')
}

function yamlDoubleQuotedScalar(value: string): string {
  return JSON.stringify(value)
}

function buildOMPEquivalenceOverrides(models: Record<string, OMPModelConfig>): string {
  const lines = Object.keys(models)
    .sort()
    .map((id) => `    ${PROVIDER_ID}/${id}: ${id}`)
  lines.push(`    ${IMAGE_PROVIDER_ID}/${DEFAULT_AGENT_MODEL_ID}-Sys: ${DEFAULT_AGENT_MODEL_ID}-image-sys`)
  return lines.join('\n')
}


function deepCloneRecord(value?: Record<string, unknown>): Record<string, unknown> {
  if (!value) return {}
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [key, deepCloneValue(item)])
  )
}

function deepCloneValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(deepCloneValue)
  if (value && typeof value === 'object') {
    return deepCloneRecord(value as Record<string, unknown>)
  }
  return value
}

function compactRecord(input: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(input).filter(([, value]) => {
      if (value === undefined || value === 0) return false
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        return Object.keys(value).length > 0
      }
      return true
    })
  )
}

function formatNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : String(value)
}
