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
import { type ReactElement, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Code2, Cpu, KeyRound, Settings2, TerminalSquare, WandSparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'
import type { SystemStatus } from '@/features/auth/types'
import { fetchTokenKeysBatch, getApiKeys, getOpenCodeOpenAIModels } from '@/features/keys/api'
import { API_KEY_STATUS } from '@/features/keys/constants'
import type { ModelOption } from '@/features/playground/types'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { CopyButton } from '@/components/copy-button'
import {
  buildAgentConfigGuideInstruction,
  buildCherryStudioConfig,
  buildContinueConfig,
  buildGenericEnvConfig,
  buildGenericJsonConfig,
  buildOmpModelsConfig,
  buildOmpSettingsConfig,
  buildOmpImageGeneratorConfig,
  buildOmpPluginInstructions,
  buildOpenAIBaseUrl,
  buildOpenCodeConfig,
  type AgentConfigGuideClient,
  type OpenCodeOpenAIModel,
} from '../lib/usage-config'

type ApiHelpKey = {
  id: number
  name: string
  key: string
}

type ConfigFile = {
  path: string
  content: string
  hint?: string
}

const REQUIRED_OPENCODE_MODEL_IDS = ['gpt-5'] as const
const REQUIRED_OMP_MODEL_IDS = ['gpt-5', 'gpt-5-mini'] as const

type MetadataState = 'loading' | 'ready' | 'unavailable'

function metadataNoticeText(
  state: MetadataState,
  missingIds: string[] | undefined,
  t: ReturnType<typeof useTranslation>['t']
): string {
  if (state === 'loading') return t('Loading AI auto-configuration metadata...')
  if (missingIds && missingIds.length > 0) {
    return t('AI auto-configuration metadata is missing required models: {{models}}', {
      models: missingIds.join(', '),
    })
  }
  return t('AI auto-configuration metadata is unavailable. Manual snippets are still shown below.')
}

function missingModelIds(
  models: Record<string, OpenCodeOpenAIModel> | undefined,
  requiredIds: readonly string[]
): string[] {
  if (!models) return [...requiredIds]
  return requiredIds.filter((id) => !models[id])
}
type ApiUsageHelpDialogProps = {
  modelValue: string
  models: ModelOption[]
  trigger?: ReactElement
}

const API_KEY_PAGE_SIZE = 50
const API_HELP_KEY_LIMIT = 50

function extractServerAddress(status: SystemStatus | null): string {
  const fromStatus =
    status?.server_address ??
    status?.serverAddress ??
    status?.data?.server_address ??
    status?.data?.serverAddress

  if (typeof fromStatus === 'string' && fromStatus.trim()) {
    return fromStatus.trim().replace(/\/+$/, '')
  }

  if (typeof window !== 'undefined') {
    return window.location.origin
  }

  return ''
}

async function fetchEnabledApiKeys(): Promise<ApiHelpKey[]> {
  const activeItems = []
  let page = 1
  let totalPages = 1

  do {
    const result = await getApiKeys({ p: page, size: API_KEY_PAGE_SIZE })
    if (!result.success) return []

    activeItems.push(
      ...(result.data?.items ?? []).filter(
        (item) => item.status === API_KEY_STATUS.ENABLED
      )
    )

    const total = result.data?.total ?? activeItems.length
    totalPages = Math.ceil(total / API_KEY_PAGE_SIZE)
    page += 1
  } while (activeItems.length < API_HELP_KEY_LIMIT && page <= totalPages)

  if (activeItems.length === 0) return []

  const visibleItems = activeItems.slice(0, API_HELP_KEY_LIMIT)
  const keyResult = await fetchTokenKeysBatch(visibleItems.map((item) => item.id))
  if (!keyResult.success || !keyResult.data?.keys) return []

  return visibleItems
    .map((item) => {
      const key = keyResult.data?.keys[item.id]
      if (!key) return null
      return {
        id: item.id,
        name: item.name,
        key,
      }
    })
    .filter((entry): entry is ApiHelpKey => entry !== null)
}

function ConfigFileBlock(props: ConfigFile) {
  const { t } = useTranslation()

  return (
    <div className='overflow-hidden rounded-xl border bg-slate-950 text-slate-100 shadow-sm dark:bg-slate-950'>
      <div className='flex items-start justify-between gap-3 border-b border-white/10 bg-white/5 px-3 py-2'>
        <div className='min-w-0'>
          <div className='truncate font-mono text-xs font-semibold text-slate-50'>
            {props.path}
          </div>
          {props.hint ? (
            <div className='mt-0.5 text-xs text-slate-400'>
              {props.hint}
            </div>
          ) : null}
        </div>
        <CopyButton
          value={props.content}
          variant='outline'
          size='sm'
          className='h-7 shrink-0 gap-1.5 border-white/15 bg-white/10 px-2 text-xs text-slate-50 hover:bg-white/15 hover:text-white'
          iconClassName='size-3.5'
          tooltip={t('Copy configuration')}
          aria-label={t('Copy configuration')}
        >
          {t('Copy')}
        </CopyButton>
      </div>
      <pre className='max-h-80 overflow-auto p-3 text-xs leading-relaxed whitespace-pre-wrap text-slate-100'>
        <code>{props.content}</code>
      </pre>
    </div>
  )
}

function ConfigFileList(props: { files: ConfigFile[] }) {
  return (
    <div className='space-y-3'>
      {props.files.map((file) => (
        <ConfigFileBlock key={file.path} {...file} />
      ))}
    </div>
  )
}

function MetadataNotice(props: { state: MetadataState; missingIds?: string[] }) {
  const { t } = useTranslation()

  if (props.state === 'ready') return null

  const text = metadataNoticeText(props.state, props.missingIds, t)

  return (
    <Alert>
      <AlertDescription>{text}</AlertDescription>
    </Alert>
  )
}

function AutoConfigCard(props: {
  client: AgentConfigGuideClient
  serverAddress: string
  apiKey: string
  state: MetadataState
  missingIds?: string[]
}) {
  const { t } = useTranslation()
  const ready = props.state === 'ready'
  const instruction = buildAgentConfigGuideInstruction(
    props.serverAddress,
    props.client,
    props.apiKey
  )
  const label = props.client === 'omp' ? 'OMP' : 'OpenCode'

  return (
    <div className='rounded-xl border bg-muted/30 p-3'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='min-w-0 space-y-1'>
          <div className='flex items-center gap-2 text-sm font-medium'>
            <WandSparkles className='size-4 text-primary' />
            {t('AI auto-configuration')}
          </div>
          <p className='text-muted-foreground text-xs'>
            {ready
              ? t('Copy a manifest URL that another AI agent can fetch to configure {{client}} automatically.', {
                  client: label,
                })
              : metadataNoticeText(props.state, props.missingIds, t)}
          </p>
        </div>
        {ready && props.apiKey ? (
          <CopyButton
            value={instruction}
            variant='outline'
            size='sm'
            className='shrink-0 gap-1.5'
            iconClassName='size-3.5'
            tooltip={t('Copy AI auto-configuration instruction')}
            aria-label={t('Copy AI auto-configuration instruction')}
          >
            {t('Copy AI instruction')}
          </CopyButton>
        ) : (
          <Button
            variant='outline'
            size='sm'
            className='shrink-0 gap-1.5'
            disabled
          >
            <WandSparkles data-icon='inline-start' />
            {t('Copy AI instruction')}
          </Button>
        )}
      </div>
    </div>
  )
}

function KeySelector(props: {
  apiKeys: ApiHelpKey[]
  selectedKeyId: string
  onSelect: (id: string) => void
}) {
  const { t } = useTranslation()

  if (props.apiKeys.length === 0) {
    return (
      <Alert>
        <AlertDescription>
          {t('Create an API key first to copy ready-to-use client configs.')}{' '}
          <Link to='/keys' className='underline underline-offset-4'>
            {t('Create API Key')}
          </Link>
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className='min-w-56 space-y-1.5'>
      <div className='text-muted-foreground text-xs font-medium'>
        {t('API Key')}
      </div>
      <ComboboxInput
        options={props.apiKeys.map((item) => ({
          value: String(item.id),
          label: item.name || `#${item.id}`,
        }))}
        value={props.selectedKeyId}
        onValueChange={props.onSelect}
        placeholder={t('Select API Key')}
        emptyText={t('No enabled API keys')}
      />
    </div>
  )
}

export function ApiUsageHelpDialog(props: ApiUsageHelpDialogProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [selectedKeyId, setSelectedKeyId] = useState('')
  const userId = useAuthStore((state) => state.auth.user?.id)
  const { status } = useStatus()
  const serverAddress = useMemo(() => extractServerAddress(status), [status])
  const apiKeysQuery = useQuery({
    queryKey: ['api-help', 'api-keys', userId],
    queryFn: fetchEnabledApiKeys,
    enabled: open && Boolean(userId),
    staleTime: 5 * 60 * 1000,
  })
  const metadataQuery = useQuery({
    queryKey: ['api-help', 'opencode-openai-models', userId],
    queryFn: async () => {
      const result = await getOpenCodeOpenAIModels()
      return result.success ? result.data : undefined
    },
    enabled: open && Boolean(userId),
    staleTime: 15 * 60 * 1000,
  })

  const apiKeys = apiKeysQuery.data ?? []
  const selectedApiKey =
    apiKeys.find((item) => String(item.id) === selectedKeyId) ?? apiKeys[0]
  const apiKey = selectedApiKey?.key ?? ''
  const selectedModel =
    props.modelValue || props.models[0]?.value || 'gpt-4o-mini'
  const baseUrl = buildOpenAIBaseUrl(serverAddress)
  const metadata = metadataQuery.data
  const openCodeModels = metadata?.models
  const ompPluginVersion = metadata?.omp_openai_provider_tools?.latest_version ?? ''
  const opencodeMissingIds = missingModelIds(openCodeModels, REQUIRED_OPENCODE_MODEL_IDS)
  const ompMissingIds = missingModelIds(openCodeModels, REQUIRED_OMP_MODEL_IDS)
  const pluginStatus = metadata?.omp_openai_provider_tools?.status
  const opencodeMetadataState: MetadataState = metadataQuery.isLoading
    ? 'loading'
    : openCodeModels && opencodeMissingIds.length === 0
      ? 'ready'
      : 'unavailable'
  const ompMetadataState: MetadataState = metadataQuery.isLoading
    ? 'loading'
    : openCodeModels &&
        ompMissingIds.length === 0 &&
        Boolean(ompPluginVersion) &&
        (pluginStatus === 'ok' || pluginStatus === 'cached')
      ? 'ready'
      : 'unavailable'

  const opencodeFiles = useMemo<ConfigFile[]>(
    () => [
      {
        path: 'opencode.json',
        content: buildOpenCodeConfig(
          serverAddress,
          apiKey,
          selectedModel,
          opencodeMetadataState === 'ready' ? openCodeModels : undefined
        ),
        hint: t('Place this file in ~/.config/opencode/opencode.json, then run opencode.'),
      },
    ],
    [apiKey, openCodeModels, opencodeMetadataState, selectedModel, serverAddress, t]
  )

  const ompFiles = useMemo<ConfigFile[]>(
    () => {
      if (ompMetadataState === 'ready') {
        return [
          {
            path: t('1. Install OMP provider tools plugin'),
            content: buildOmpPluginInstructions(ompPluginVersion),
            hint: t('Run these commands before using provider-native web search or image generation.'),
          },
          {
            path: '~/.omp/agent/models.yml',
            content: buildOmpModelsConfig(
              serverAddress,
              apiKey,
              selectedModel,
              openCodeModels,
              ompPluginVersion
            ),
            hint: t('Register this gateway as an OMP OpenAI Responses provider.'),
          },
          {
            path: '~/.omp/agent/config.yml',
            content: buildOmpSettingsConfig(selectedModel, openCodeModels),
            hint: t('Point OMP model roles at the gateway model.'),
          },
          {
            path: '~/.omp/agent/agents/image-generator.md',
            content: buildOmpImageGeneratorConfig(),
            hint: t('Optional image-generation subagent template for OMP.'),
          },
        ]
      }

      return [
        {
          path: '~/.omp/agent/models.yml',
          content: buildOmpModelsConfig(serverAddress, apiKey, selectedModel),
          hint: t('Register this gateway as an OMP OpenAI Responses provider.'),
        },
        {
          path: '~/.omp/agent/config.yml',
          content: buildOmpSettingsConfig(selectedModel),
          hint: t('Point OMP model roles at the gateway model.'),
        },
      ]
    },
    [apiKey, ompMetadataState, ompPluginVersion, openCodeModels, selectedModel, serverAddress, t]
  )

  const genericFiles = useMemo<ConfigFile[]>(
    () => [
      {
        path: '.env',
        content: buildGenericEnvConfig(serverAddress, apiKey),
      },
      {
        path: t('OpenAI-compatible JSON'),
        content: buildGenericJsonConfig(serverAddress, apiKey),
      },
    ],
    [apiKey, serverAddress, t]
  )

  const appFiles = useMemo<ConfigFile[]>(
    () => [
      {
        path: t('Cherry Studio provider import payload'),
        content: buildCherryStudioConfig(serverAddress, apiKey),
      },
      {
        path: t('Continue config snippet'),
        content: buildContinueConfig(serverAddress, apiKey, selectedModel),
      },
    ],
    [apiKey, selectedModel, serverAddress, t]
  )

  const defaultTrigger = (
    <Button variant='outline' size='sm' className='gap-1.5'>
      <Code2 className='size-4' />
      {t('API Help')}
    </Button>
  )

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={props.trigger ?? defaultTrigger} />
      <DialogContent className='flex max-h-[92vh] min-h-0 max-w-[calc(100%-1rem)] grid-rows-none flex-col gap-0 overflow-hidden p-0 sm:max-w-5xl'>
        <DialogHeader className='p-4 pb-3 sm:p-5 sm:pb-4'>
          <DialogTitle>{t('API Usage Help')}</DialogTitle>
          <DialogDescription>
            {t(
              'Copy ready-to-use OpenAI-compatible configs for Playground, OpenCode, OMP, and common AI apps.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='border-y px-4 py-3 sm:px-5'>
          <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
            <div className='min-w-0 space-y-1 text-sm'>
              <div className='text-muted-foreground flex items-center gap-2'>
                <TerminalSquare className='size-4' />
                <span>{t('Base URL')}</span>
              </div>
              <div className='truncate font-mono text-xs'>{baseUrl}</div>
            </div>
            <KeySelector
              apiKeys={apiKeys}
              selectedKeyId={selectedApiKey ? String(selectedApiKey.id) : ''}
              onSelect={setSelectedKeyId}
            />
          </div>
        </div>

        <ScrollArea className='min-h-0 flex-1'>
          <div className='p-4 sm:p-5'>
            <Tabs defaultValue='opencode' className='gap-4'>
              <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
                <TabsList className='h-auto max-w-full flex-wrap justify-start'>
                  <TabsTrigger value='opencode' className='gap-1.5'>
                    <TerminalSquare className='size-4' />
                    OpenCode
                  </TabsTrigger>
                  <TabsTrigger value='omp' className='gap-1.5'>
                    <Settings2 className='size-4' />
                    OMP
                  </TabsTrigger>
                  <TabsTrigger value='generic'>{t('Generic')}</TabsTrigger>
                  <TabsTrigger value='apps'>{t('Common Apps')}</TabsTrigger>
                </TabsList>
                <div className='flex items-center gap-2 rounded-lg border bg-muted/30 px-3 py-1.5 text-xs text-muted-foreground'>
                  <Cpu className='size-3.5' />
                  {t('AI auto-configuration uses models.dev metadata when available.')}
                </div>
              </div>

              <TabsContent value='opencode' className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Use the Playground-selected model in OpenCode through an OpenAI-compatible provider.'
                  )}
                </p>
                <AutoConfigCard
                  client='opencode'
                  serverAddress={serverAddress}
                  apiKey={apiKey}
                  state={opencodeMetadataState}
                  missingIds={opencodeMissingIds}
                />
                <MetadataNotice
                  state={opencodeMetadataState}
                  missingIds={opencodeMissingIds}
                />
                <ConfigFileList files={opencodeFiles} />
              </TabsContent>

              <TabsContent value='omp' className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Register the same OpenAI Responses endpoint in Oh My Pi and map model roles to it.'
                  )}
                </p>
                <AutoConfigCard
                  client='omp'
                  serverAddress={serverAddress}
                  apiKey={apiKey}
                  state={ompMetadataState}
                  missingIds={ompMissingIds}
                />
                <MetadataNotice state={ompMetadataState} missingIds={ompMissingIds} />
                <ConfigFileList files={ompFiles} />
              </TabsContent>

              <TabsContent value='generic' className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Use these values in SDKs, scripts, Chatbox, LobeChat, NextChat, or any OpenAI-compatible client.'
                  )}
                </p>
                <ConfigFileList files={genericFiles} />
              </TabsContent>

              <TabsContent value='apps' className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'These snippets replace the old standalone application guides and keep setup next to Playground testing.'
                  )}
                </p>
                <ConfigFileList files={appFiles} />
              </TabsContent>
            </Tabs>
          </div>
        </ScrollArea>

        <DialogFooter className='shrink-0 gap-2'>
          <Button variant='outline' render={<Link to='/keys' />}>
            <KeyRound data-icon='inline-start' />
            {t('Create API Key')}
          </Button>
          <Button onClick={() => setOpen(false)}>{t('Close')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
