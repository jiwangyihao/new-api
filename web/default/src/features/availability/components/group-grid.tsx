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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TitledCard } from '@/components/ui/titled-card'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { observedState } from '../lib/presentation'
import type { AvailabilityGroup, AvailabilityModel } from '../types'
import { HistoryTimeline } from './history-timeline'
import { MetricDetails } from './metric-details'

function GroupCard(props: {
  group: AvailabilityGroup
  expanded: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  const group = props.group
  const affected = group.models.filter(
    (model) =>
      model.active &&
      (observedState(model.current) === 'unhealthy' ||
        observedState(model.current) === 'degraded')
  )
  return (
    <TitledCard
      title={
        <span className='[overflow-wrap:anywhere] break-words'>
          {group.name}
        </span>
      }
      description={group.description || undefined}
      descriptionClassName='break-words [overflow-wrap:anywhere]'
      className='h-full min-w-0'
      contentClassName='flex flex-col gap-4'
    >
      {affected.length > 0 && (
        <Alert>
          <AlertDescription className='flex flex-col gap-1'>
            <StatusBadge
              copyable={false}
              variant={
                affected.some(
                  (model) => observedState(model.current) === 'unhealthy'
                )
                  ? 'danger'
                  : 'warning'
              }
              label={t('Current models need attention')}
            />
            <span className='[overflow-wrap:anywhere] break-words'>
              {affected.map((model) => model.name).join(', ')}
            </span>
          </AlertDescription>
        </Alert>
      )}
      <MetricDetails current={group.current} summary={group.summary} />
      <HistoryTimeline buckets={group.buckets} />
      <Button
        variant='outline'
        className='w-full'
        id={`availability-toggle-${group.id}`}
        aria-label={`${props.expanded ? t('Hide models') : t('Show models')}: ${group.name}`}
        aria-expanded={props.expanded}
        aria-controls={`availability-models-${group.id}`}
        onClick={props.onToggle}
      >
        {props.expanded ? t('Hide models') : t('Show models')}
        <span className='sr-only'>: {group.name}</span>
      </Button>
    </TitledCard>
  )
}
function ModelCard(props: { model: AvailabilityModel }) {
  const { t } = useTranslation()
  return (
    <TitledCard
      title={
        <span className='[overflow-wrap:anywhere] break-words'>
          {props.model.name}
        </span>
      }
      titleClassName='text-base sm:text-base'
      description={
        !props.model.active ? (
          <Badge variant='secondary'>{t('No longer offered')}</Badge>
        ) : undefined
      }
      className='min-w-0'
      contentClassName='flex flex-col gap-4'
    >
      <MetricDetails
        current={props.model.current}
        summary={props.model.summary}
      />
      <HistoryTimeline buckets={props.model.buckets} />
    </TitledCard>
  )
}
function ModelPanel(props: { group: AvailabilityGroup }) {
  const { t } = useTranslation()
  const active = props.group.models.filter((model) => model.active)
  const historical = props.group.models.filter((model) => !model.active)
  return (
    <div className='flex flex-col gap-4 rounded-lg border p-3 sm:p-5'>
      <h2 className='font-medium [overflow-wrap:anywhere] break-words'>
        {t('Models in {{group}}', { group: props.group.name })}
      </h2>
      {active.length === 0 ? (
        <EmptyState
          className='min-h-24'
          title={t('No currently offered models')}
        />
      ) : (
        <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
          {active.map((model) => (
            <ModelCard key={model.name} model={model} />
          ))}
        </div>
      )}
      {historical.length > 0 && (
        <section
          aria-label={t('Historical models')}
          className='flex flex-col gap-3'
        >
          <h3 className='text-sm font-medium'>{t('Historical models')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t('No longer offered; excluded from current model alerts.')}
          </p>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            {historical.map((model) => (
              <ModelCard key={model.name} model={model} />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
export function GroupGrid(props: { groups: AvailabilityGroup[] }) {
  const [expanded, setExpanded] = useState<number | null>(null)
  const rows: AvailabilityGroup[][] = []
  for (let index = 0; index < props.groups.length; index += 2)
    rows.push(props.groups.slice(index, index + 2))
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      {rows.map((row) => (
        <div
          key={row[0].id}
          className='grid min-w-0 grid-cols-1 items-stretch gap-4 md:grid-cols-2'
        >
          {row.map((group, index) => (
            <div
              key={group.id}
              className={cn(
                'min-w-0',
                index === 0 ? 'order-1' : 'order-3 md:order-2'
              )}
            >
              <GroupCard
                group={group}
                expanded={expanded === group.id}
                onToggle={() =>
                  setExpanded(expanded === group.id ? null : group.id)
                }
              />
            </div>
          ))}
          {row.map((group, index) => (
            <section
              key={`models-${group.id}`}
              id={`availability-models-${group.id}`}
              aria-labelledby={`availability-toggle-${group.id}`}
              hidden={expanded !== group.id}
              className={cn(
                'min-w-0 md:order-3 md:col-span-2',
                index === 0 ? 'order-2' : 'order-4'
              )}
            >
              {expanded === group.id && <ModelPanel group={group} />}
            </section>
          ))}
        </div>
      ))}
    </div>
  )
}
