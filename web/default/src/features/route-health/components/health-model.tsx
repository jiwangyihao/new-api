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
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { StatusBadge } from '@/components/status-badge'
import type { ModelHealthGroup } from '../lib/health-overview'
import type { RouteHealthRow } from '../types'
import { HealthStateBadge } from './health-state-badge'
import { AttemptDiagnostics } from './resource-details'

export function HealthModel(props: {
  model: ModelHealthGroup
  onDetails: (row: RouteHealthRow) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const model = props.model
  const row = model.row
  const resource = row.resources.find((item) => item.scope === 'model')
  const aliases = model.aliases.filter((alias) => alias !== model.name)
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='min-w-0 rounded-lg border'
    >
      <CollapsibleTrigger
        aria-label={t('Show model diagnostics for {{model}}', {
          model: model.name,
        })}
        className='hover:bg-muted/50 focus-visible:ring-ring flex w-full flex-wrap items-start gap-3 rounded-lg p-3 text-left outline-none focus-visible:ring-2'
      >
        <ChevronRight
          aria-hidden='true'
          className={`mt-1 size-4 shrink-0 transition-transform ${open ? 'rotate-90' : ''}`}
        />
        <span className='min-w-0 flex-1'>
          <span className='block font-medium break-all'>
            {model.name || '-'}
          </span>
          {aliases.length > 0 && (
            <span className='text-muted-foreground block text-xs break-all'>
              {t('Client model aliases')}: {aliases.join(', ')}
            </span>
          )}
        </span>
        <span className='flex flex-wrap gap-1 text-xs'>
          <HealthStateBadge state={row.state} />
          {model.attention && (
            <StatusBadge copyable={false} variant='warning'>
              {t('Needs attention')}
            </StatusBadge>
          )}
        </span>
      </CollapsibleTrigger>
      <div className='flex flex-wrap gap-x-4 gap-y-1 px-3 pb-3 text-xs tabular-nums'>
        <span>
          {t('Successes')}: {formatNumber(resource?.successes)}
        </span>
        <span>
          {t('Failures')}: {formatNumber(resource?.failures)}
        </span>
        <span>
          {t('Consecutive failures')}:{' '}
          {formatNumber(resource?.consecutive_failures)}
        </span>
        <span>
          {t('In flight')}: {formatNumber(resource?.inflight)}
        </span>
        <span>
          {t('Ready / total credentials')}:{' '}
          {formatNumber(row.ready_credentials)} /{' '}
          {formatNumber(row.total_credentials)}
        </span>
      </div>
      <CollapsibleContent>
        {open && (
          <div className='min-w-0 space-y-3 border-t p-3'>
            <AttemptDiagnostics attempts={row.attempts} />
            <Button
              size='sm'
              variant='outline'
              onClick={() => props.onDetails(row)}
            >
              {t('Details')}
            </Button>
          </div>
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}
