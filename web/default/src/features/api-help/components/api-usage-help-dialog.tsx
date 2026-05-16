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
import { Code2, KeyRound, Settings2, TerminalSquare } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'
import type { SystemStatus } from '@/features/auth/types'
import { fetchTokenKeysBatch, getApiKeys } from '@/features/keys/api'
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
  buildCherryStudioConfig,
  buildContinueConfig,
  buildGenericEnvConfig,
  buildGenericJsonConfig,
  buildOmpModelsConfig,
  buildOmpSettingsConfig,
  buildOpenAIBaseUrl,
  buildOpenCodeConfig,
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
    <div className='overflow-hidden rounded-xl border bg-card'>
      <div className='bg-muted/60 flex items-center justify-between gap-3 border-b px-3 py-2'>
        <div className='min-w-0'>
          <div className='truncate font-mono text-xs font-medium'>
            {props.path}
          </div>
          {props.hint ? (
            <div className='text-muted-foreground mt-0.5 text-xs'>
              {props.hint}
            </div>
          ) : null}
        </div>
        <CopyButton
          value={props.content}
          variant='outline'
          size='sm'
          className='h-7 gap-1.5 px-2 text-xs'
          iconClassName='size-3.5'
          tooltip={t('Copy configuration')}
          aria-label={t('Copy configuration')}
        >
          {t('Copy')}
        </CopyButton>
      </div>
      <pre className='max-h-72 overflow-auto p-3 text-xs leading-relaxed whitespace-pre-wrap'>
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

  const apiKeys = apiKeysQuery.data ?? []
  const selectedApiKey =
    apiKeys.find((item) => String(item.id) === selectedKeyId) ?? apiKeys[0]
  const apiKey = selectedApiKey?.key ?? ''
  const selectedModel =
    props.modelValue || props.models[0]?.value || 'gpt-4o-mini'
  const baseUrl = buildOpenAIBaseUrl(serverAddress)

  const opencodeFiles = useMemo<ConfigFile[]>(
    () => [
      {
        path: 'opencode.json',
        content: buildOpenCodeConfig(serverAddress, apiKey, selectedModel),
        hint: t('Place this file in your project root, then run opencode.'),
      },
    ],
    [apiKey, selectedModel, serverAddress, t]
  )

  const ompFiles = useMemo<ConfigFile[]>(
    () => [
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
    ],
    [apiKey, selectedModel, serverAddress, t]
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
      <DialogContent className='max-h-[92vh] gap-0 p-0 sm:max-w-4xl'>
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

        <ScrollArea className='max-h-[calc(92vh-14rem)]'>
          <div className='p-4 sm:p-5'>
            <Tabs defaultValue='opencode' className='gap-4'>
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

              <TabsContent value='opencode' className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Use the Playground-selected model in OpenCode through an OpenAI-compatible provider.'
                  )}
                </p>
                <ConfigFileList files={opencodeFiles} />
              </TabsContent>

              <TabsContent value='omp' className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Register the same OpenAI Responses endpoint in Oh My Pi and map model roles to it.'
                  )}
                </p>
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

        <DialogFooter className='gap-2'>
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
