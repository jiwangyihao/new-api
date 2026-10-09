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
import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { StatusBadge } from '@/components/status-badge'
import type {
  ChannelHealthGroup,
  ModelHealthGroup,
} from '../lib/health-overview'
import type { RouteHealthRow } from '../types'
import { HealthModel } from './health-model'

export function ChannelHealthCard(props: {
  channel: ChannelHealthGroup
  models: ModelHealthGroup[]
  searching: boolean
  enabled: boolean
  onDetails: (row: RouteHealthRow) => void
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(props.searching)
  const channel = props.channel
  const open = expanded
  const issueModels = channel.models.filter((model) => model.attention)
  let label = t('Unobserved')
  let variant: 'neutral' | 'warning' | 'info' = 'neutral'
  if (channel.status === 2) label = t('Manually disabled')
  else if (channel.attentionModels > 0) {
    label = t('Needs attention')
    variant = 'warning'
  } else if (!props.enabled) label = t('Health protection disabled')
  else if (channel.observedModels > 0) {
    label = t('Recent activity')
    variant = 'info'
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setExpanded}
      data-channel-id={channel.id}
      className={cn(
        'bg-card overflow-hidden rounded-xl border',
        channel.attentionModels > 0 && 'border-warning/50'
      )}
    >
      <CollapsibleTrigger
        aria-label={t('Show models for {{channel}}', { channel: channel.name })}
        className='hover:bg-muted/40 focus-visible:ring-ring flex w-full items-start gap-3 p-4 text-left outline-none focus-visible:ring-2 sm:items-center'
      >
        <ChevronRight
          aria-hidden='true'
          className={cn(
            'text-muted-foreground mt-1 size-4 shrink-0 transition-transform sm:mt-0',
            open && 'rotate-90'
          )}
        />
        <span className='min-w-0 flex-1 space-y-2'>
          <span className='flex flex-wrap items-center gap-x-3 gap-y-1'>
            <span className='text-base font-semibold break-all'>
              {channel.name}
            </span>
            <span className='text-muted-foreground text-xs'>#{channel.id}</span>
            <StatusBadge copyable={false} variant={variant}>
              {label}
            </StatusBadge>
          </span>
          <span className='text-muted-foreground flex flex-wrap gap-x-5 gap-y-1 text-xs tabular-nums'>
            <span>
              {t('Models')}: {formatNumber(channel.models.length)}
            </span>
            <span className={channel.attentionModels ? 'text-warning' : ''}>
              {t('Models needing attention')}:{' '}
              {formatNumber(channel.attentionModels)}
            </span>
            <span>
              {t('Observed models')}: {formatNumber(channel.observedModels)}
            </span>
          </span>
          {issueModels.length > 0 && (
            <span className='text-warning block truncate text-xs'>
              {issueModels
                .slice(0, 3)
                .map((model) => model.name)
                .join(' · ')}
              {issueModels.length > 3 &&
                ` +${formatNumber(issueModels.length - 3)}`}
            </span>
          )}
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        {open && (
          <div className='space-y-2 border-t p-3 sm:p-4'>
            <p className='text-muted-foreground pb-1 text-xs'>
              {t('Showing {{shown}} of {{total}} models', {
                shown: formatNumber(props.models.length),
                total: formatNumber(channel.models.length),
              })}
            </p>
            {props.models.map((model) => (
              <HealthModel
                key={model.id}
                model={model}
                onDetails={props.onDetails}
              />
            ))}
          </div>
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}
