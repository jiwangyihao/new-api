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
import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Bot,
  Code2,
  Laptop,
  MessageSquare,
  Monitor,
  Puzzle,
  TerminalSquare,
  Workflow,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { fetchTokenKeysBatch, getApiKeys } from '@/features/keys/api'
import { API_KEY_STATUS } from '@/features/keys/constants'
import { AppGuideCard } from './components/app-guide-card'
import {
  buildCCSwitchImportUrl,
  buildCherryStudioConfig,
  buildContinueConfig,
  buildOpenAIBaseUrl,
  buildOpenAICompatibleConfig,
  buildOpenAICompatibleEnv,
  buildQueryImportUrl,
  encodeImportConfig,
} from './lib'
import type { AppGuide } from './types'

function extractServerAddress(status: Record<string, unknown> | null): string {
  const data = status?.data as Record<string, unknown> | undefined
  const fromStatus =
    status?.server_address ?? status?.serverAddress ?? data?.server_address ?? data?.serverAddress

  if (typeof fromStatus === 'string' && fromStatus.trim()) {
    return fromStatus.trim().replace(/\/+$/, '')
  }

  if (typeof window !== 'undefined') {
    return window.location.origin
  }

  return ''
}

async function fetchEnabledApiKeys(): Promise<
  { id: number; name: string; key: string }[]
> {
  const result = await getApiKeys({ p: 1, size: 50 })
  const activeItems = (result.data?.items ?? []).filter(
    (item) => item.status === API_KEY_STATUS.ENABLED
  )
  if (!result.success || activeItems.length === 0) return []

  const keyResult = await fetchTokenKeysBatch(activeItems.map((item) => item.id))
  if (!keyResult.success || !keyResult.data?.keys) return []

  return activeItems
    .map((item) => {
      const key = keyResult.data?.keys[item.id]
      if (!key) return null
      return {
        id: item.id,
        name: item.name,
        key,
      }
    })
    .filter(
      (entry): entry is { id: number; name: string; key: string } =>
        entry !== null
    )
}

export function AppGuides() {
  const { t } = useTranslation()
  const [selectedKeyId, setSelectedKeyId] = useState('')
  const userId = useAuthStore((state) => state.auth.user?.id)
  const { status } = useStatus()
  const serverAddress = useMemo(
    () => extractServerAddress(status as Record<string, unknown> | null),
    [status]
  )
  const baseUrl = buildOpenAIBaseUrl(serverAddress)

  const apiKeysQuery = useQuery({
    queryKey: ['app-guides', 'api-keys', userId],
    queryFn: fetchEnabledApiKeys,
    enabled: Boolean(userId),
    staleTime: 5 * 60 * 1000,
  })
  const apiKeys = apiKeysQuery.data ?? []
  const selectedApiKey =
    apiKeys.find((item) => String(item.id) === selectedKeyId) ?? apiKeys[0]
  const apiKey = selectedApiKey?.key ?? ''
  const genericConfig = buildOpenAICompatibleConfig(serverAddress, apiKey)
  const envConfig = buildOpenAICompatibleEnv(serverAddress, apiKey)
  const cherryConfig = buildCherryStudioConfig(serverAddress, apiKey)
  const encodedCherryConfig = encodeImportConfig(cherryConfig)
  const ccSwitchLinks = [
    {
      id: 'ccswitch-claude',
      label: t('Import to CC Switch'),
      href: buildCCSwitchImportUrl({
        app: 'claude',
        name: 'new-api',
        serverAddress,
        apiKey,
        model: 'gpt-4o-mini',
      }),
      requiresKey: true,
    },
  ]

  const guides: AppGuide[] = [
    {
      id: 'cherry-studio',
      name: 'Cherry Studio',
      description: t('Import the OpenAI-compatible endpoint into Cherry Studio.'),
      icon: Laptop,
      configTitle: t('One-click import payload'),
      configValue: JSON.stringify(cherryConfig, null, 2),
      importLinks: [
        {
          id: 'cherry-studio-import',
          label: t('Import to Cherry Studio'),
          href: `cherrystudio://provider/import?data=${encodedCherryConfig}`,
          requiresKey: true,
        },
      ],
    },
    {
      id: 'chatbox',
      name: 'Chatbox',
      description: t('Use a custom OpenAI-compatible provider in Chatbox.'),
      icon: MessageSquare,
      configTitle: t('OpenAI-compatible configuration'),
      configValue: genericConfig,
      importLinks: [
        {
          id: 'chatbox-import',
          label: t('Import to Chatbox'),
          href: buildQueryImportUrl('chatbox://import', serverAddress, apiKey),
          requiresKey: true,
        },
      ],
    },
    {
      id: 'lobechat',
      name: 'LobeChat',
      description: t('Configure LobeChat with the OpenAI endpoint and API key.'),
      icon: Bot,
      configTitle: t('Environment variables'),
      configValue: envConfig,
      importLinks: [
        {
          id: 'lobechat-import',
          label: t('Import to LobeChat'),
          href: buildQueryImportUrl('lobechat://settings', serverAddress, apiKey),
          requiresKey: true,
        },
      ],
    },
    {
      id: 'nextchat',
      name: 'NextChat',
      description: t('Paste the Base URL and API key into NextChat settings.'),
      icon: Monitor,
      configTitle: t('OpenAI-compatible configuration'),
      configValue: genericConfig,
      importLinks: [
        {
          id: 'nextchat-import',
          label: t('Import to NextChat'),
          href: buildQueryImportUrl('nextchat://settings', serverAddress, apiKey),
          requiresKey: true,
        },
      ],
    },
    {
      id: 'open-webui',
      name: 'Open WebUI',
      description: t('Add an OpenAI API connection in Open WebUI.'),
      icon: Workflow,
      configTitle: t('Environment variables'),
      configValue: envConfig,
      importLinks: [
        {
          id: 'open-webui-import',
          label: t('Import to Open WebUI'),
          href: buildQueryImportUrl('openwebui://settings', serverAddress, apiKey),
          requiresKey: true,
        },
      ],
    },
    {
      id: 'continue',
      name: 'Continue',
      description: t('Use Continue with an OpenAI-compatible chat model.'),
      icon: Code2,
      configTitle: t('Continue config snippet'),
      configValue: buildContinueConfig(serverAddress, apiKey),
      importLinks: [
        {
          id: 'continue-import',
          label: t('Import to Continue'),
          href: buildQueryImportUrl('continue://config', serverAddress, apiKey),
          requiresKey: true,
        },
      ],
    },
    {
      id: 'cline',
      name: 'Cline',
      description: t('Select OpenAI-compatible provider in Cline and paste these values.'),
      icon: Puzzle,
      configTitle: t('OpenAI-compatible configuration'),
      configValue: genericConfig,
      importLinks: [
        {
          id: 'cline-import',
          label: t('Import to Cline'),
          href: buildQueryImportUrl('cline://settings', serverAddress, apiKey),
          requiresKey: true,
        },
      ],
    },
    {
      id: 'claude-code',
      name: 'Claude Code / CC Switch',
      description: t('Use environment variables for OpenAI-compatible command-line clients.'),
      icon: TerminalSquare,
      configTitle: t('Environment variables'),
      configValue: envConfig,
      importLinks: ccSwitchLinks,
    },
  ]

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Application Guides')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Copy Base URL and API key settings for common AI applications.')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
          <Alert>
            <AlertDescription>
              {t('Base URL')}: <span className='font-mono'>{baseUrl}</span>
              <span className='mx-2 text-muted-foreground'>·</span>
              {t('API Key')}: {selectedApiKey?.name ?? t('No enabled API keys')}
            </AlertDescription>
          </Alert>

          <div className='max-w-md space-y-2'>
            <div className='text-sm font-medium'>{t('Select API Key')}</div>
            <ComboboxInput
              options={apiKeys.map((item) => ({
                value: String(item.id),
                label: item.name || `#${item.id}`,
              }))}
              value={selectedApiKey ? String(selectedApiKey.id) : ''}
              onValueChange={setSelectedKeyId}
              placeholder={t('Select API Key')}
              emptyText={t('No enabled API keys')}
            />
          </div>

          <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-3'>
            {guides.map((guide) => (
              <AppGuideCard
                key={guide.id}
                guide={guide}
                hasApiKey={Boolean(apiKey)}
              />
            ))}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
