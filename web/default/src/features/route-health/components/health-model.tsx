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
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { StatusBadge } from '@/components/status-badge'
import {
  routeNeedsAttention,
  type ModelHealthGroup,
} from '../lib/health-overview'
import type { RouteHealthRow } from '../types'
import { HealthStateBadge } from './health-state-badge'

export function HealthModel(props: {
  model: ModelHealthGroup
  onDetails: (row: RouteHealthRow) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const model = props.model
  const aliases = model.aliases.filter((alias) => alias !== model.name)
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='rounded-lg border'
    >
      <CollapsibleTrigger
        aria-label={t('Show operations for {{model}}', { model: model.name })}
        className='hover:bg-muted/50 focus-visible:ring-ring flex w-full items-center gap-3 rounded-lg p-3 text-left outline-none focus-visible:ring-2'
      >
        <ChevronRight
          aria-hidden='true'
          className={`size-4 shrink-0 transition-transform ${open ? 'rotate-90' : ''}`}
        />
        <span className='min-w-0 flex-1'>
          <span className='block font-medium break-all'>
            {model.name || '-'}
          </span>
          {aliases.length > 0 && (
            <span
              className='text-muted-foreground block truncate text-xs'
              title={aliases.join(', ')}
            >
              {aliases.join(', ')}
            </span>
          )}
        </span>
        <span className='flex shrink-0 flex-col items-end gap-1 text-xs'>
          {model.attention ? (
            <StatusBadge copyable={false} variant='warning'>
              {t('Needs attention')}
            </StatusBadge>
          ) : (
            <span className='text-muted-foreground'>
              {model.observed ? t('Recent activity') : t('Unobserved')}
            </span>
          )}
          <span className='text-muted-foreground'>
            {t('{{count}} operations', { count: model.routes.length })}
          </span>
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        {open && (
          <ul className='space-y-2 border-t p-3'>
            {model.routes.map((route) => {
              const resource = route.resources.find(
                (item) => item.scope === 'route'
              )
              return (
                <li
                  key={route.id}
                  className='bg-muted/30 flex flex-wrap items-center gap-3 rounded-md p-3'
                >
                  <div className='min-w-0 flex-1 space-y-2'>
                    <div className='flex flex-wrap items-center gap-x-3 gap-y-1'>
                      <code className='text-xs break-all'>
                        {route.operation}
                      </code>
                      <HealthStateBadge state={route.state} />
                      {routeNeedsAttention(route) &&
                        route.state === 'healthy' && (
                          <StatusBadge copyable={false} variant='warning'>
                            {t('Needs attention')}
                          </StatusBadge>
                        )}
                    </div>
                    <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs tabular-nums'>
                      <span>
                        {t('Successes')}: {formatNumber(resource?.successes)}
                      </span>
                      <span>
                        {t('Failures')}: {formatNumber(resource?.failures)}
                      </span>
                      <span>
                        {t('In flight')}: {formatNumber(resource?.inflight)}
                      </span>
                      <span>
                        {t('Ready / total credentials')}:{' '}
                        {formatNumber(route.ready_credentials)} /{' '}
                        {formatNumber(route.total_credentials)}
                      </span>
                      {!!resource?.opened_until && (
                        <span>
                          {t('Cooling until')}:{' '}
                          {formatTimestampToDate(resource.opened_until)}
                        </span>
                      )}
                    </div>
                  </div>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => props.onDetails(route)}
                  >
                    {t('Details')}
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}
