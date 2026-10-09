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
import { useTranslation } from 'react-i18next'
import { formatNumber } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { EmptyState } from '@/components/empty-state'
import {
  groupRouteHealth,
  selectChannelHealth,
  type HealthView,
} from '../lib/health-overview'
import type { RouteHealthReport } from '../types'
import { ChannelHealthCard } from './channel-health-card'
import { ResourceDetails } from './resource-details'

export function ChannelHealthOverview(props: { report: RouteHealthReport }) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [view, setView] = useState<HealthView>('all')
  const [showManuallyDisabled, setShowManuallyDisabled] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const channels = useMemo(
    () => groupRouteHealth(props.report.rows),
    [props.report.rows]
  )
  const visible = useMemo(
    () => selectChannelHealth(channels, { search, view, showManuallyDisabled }),
    [channels, search, view, showManuallyDisabled]
  )
  const included = channels.filter(
    (channel) => showManuallyDisabled || channel.status !== 2
  )
  const hiddenCount = channels.length - included.length
  const attentionCount = included.filter(
    (channel) => channel.attentionModels > 0
  ).length
  const unobservedCount = included.filter((channel) =>
    channel.models.some((model) => !model.observed && !model.attention)
  ).length
  const selectedRow = props.report.rows.find(
    (row) =>
      row.id === selectedId &&
      (showManuallyDisabled || row.channel_status !== 2)
  )
  const scopes: { id: HealthView; label: string; count: number }[] = [
    { id: 'all', label: t('All visible channels'), count: included.length },
    { id: 'attention', label: t('Needs attention'), count: attentionCount },
    {
      id: 'unobserved',
      label: t('Channels with unobserved models'),
      count: unobservedCount,
    },
  ]

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <Input
          className='sm:max-w-sm'
          aria-label={t('Search channels, models, or operations...')}
          placeholder={t('Search channels, models, or operations...')}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <div className='flex items-center gap-2'>
          <Switch
            id='health-show-disabled'
            checked={showManuallyDisabled}
            onCheckedChange={setShowManuallyDisabled}
          />
          <Label htmlFor='health-show-disabled' className='cursor-pointer'>
            {t('Show manually disabled channels')}
          </Label>
        </div>
      </div>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div
          className='flex flex-wrap gap-2'
          role='group'
          aria-label={t('Channel health filters')}
        >
          {scopes.map((scope) => (
            <Button
              key={scope.id}
              variant={view === scope.id ? 'secondary' : 'ghost'}
              size='sm'
              aria-pressed={view === scope.id}
              onClick={() => setView(scope.id)}
            >
              {scope.label}
              <span className='text-muted-foreground tabular-nums'>
                {formatNumber(scope.count)}
              </span>
            </Button>
          ))}
        </div>
        <span className='text-muted-foreground text-xs'>
          {t('{{count}} channels shown', { count: visible.length })}
          {hiddenCount > 0 && (
            <>
              {' '}
              ·{' '}
              {t('{{count}} manually disabled channels hidden', {
                count: hiddenCount,
              })}
            </>
          )}
        </span>
      </div>
      {visible.length ? (
        <div className='space-y-3' aria-label={t('Channel health overview')}>
          {visible.map(({ channel, models }) => (
            <ChannelHealthCard
              key={`${channel.id}:${view}:${search.trim()}`}
              channel={channel}
              models={models}
              enabled={props.report.enabled}
              searching={search.trim().length > 0 || view === 'attention'}
              onDetails={(row) => setSelectedId(row.id)}
            />
          ))}
        </div>
      ) : (
        <EmptyState
          bordered
          title={
            view === 'attention' && !search
              ? t('No channels need attention')
              : t('No matching channels')
          }
          description={t(
            'Search covers every model and operation. Manually disabled channels stay hidden until enabled above.'
          )}
          action={
            <Button
              variant='outline'
              onClick={() => {
                setSearch('')
                setView('all')
              }}
            >
              {t('Reset filters')}
            </Button>
          }
        />
      )}
      <ResourceDetails
        row={selectedRow}
        generatedAt={props.report.generated_at}
        onClose={() => setSelectedId(null)}
      />
    </div>
  )
}
