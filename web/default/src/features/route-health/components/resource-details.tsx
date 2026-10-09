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
import { useTranslation } from 'react-i18next'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { RouteHealthResource, RouteHealthRow } from '../types'
import { HealthStateBadge } from './health-state-badge'

function ResourceCard(props: { resource: RouteHealthResource }) {
  const { t } = useTranslation()
  const resource = props.resource
  const scopeLabels = {
    channel: t('Channel scope'),
    route: t('Route scope'),
    credential: t('Credential scope'),
  }
  const fields = [
    [
      t('Credential number'),
      resource.scope === 'credential'
        ? formatNumber(resource.credential_index)
        : '-',
    ],
    [t('Successes'), formatNumber(resource.successes)],
    [t('Failures'), formatNumber(resource.failures)],
    [t('Consecutive failures'), formatNumber(resource.consecutive_failures)],
    [t('In flight'), formatNumber(resource.inflight)],
    [t('Cooling until'), formatTimestampToDate(resource.opened_until)],
    [t('Next probe'), formatTimestampToDate(resource.next_probe)],
    [t('Probe in progress'), resource.probe ? t('Yes') : t('No')],
    [t('Recovery successes'), formatNumber(resource.recovery_successes)],
    [t('Degraded'), resource.degraded ? t('Yes') : t('No')],
  ]
  return (
    <section className='space-y-3 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='font-medium'>{scopeLabels[resource.scope]}</h3>
        <HealthStateBadge state={resource.state} />
      </div>
      <dl className='grid grid-cols-2 gap-3 text-xs sm:grid-cols-3'>
        {fields.map(([label, value]) => (
          <div key={label}>
            <dt className='text-muted-foreground'>{label}</dt>
            <dd className='mt-1 break-words tabular-nums'>{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

export function ResourceDetails(props: {
  row: RouteHealthRow | undefined
  generatedAt?: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const row = props.row
  return (
    <Dialog
      open={row != null}
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
    >
      <DialogContent
        showCloseButton={false}
        className='max-h-[90dvh] overflow-y-auto sm:max-w-2xl'
      >
        <DialogHeader>
          <DialogTitle>{t('Route health details')}</DialogTitle>
          <DialogDescription>
            {t(
              'Channel and credential states may be shared across multiple model routes; their counters are not route request totals.'
            )}
          </DialogDescription>
        </DialogHeader>
        {row && (
          <>
            <dl className='grid gap-3 text-sm sm:grid-cols-2'>
              <div>
                <dt className='text-muted-foreground'>{t('Channel')}</dt>
                <dd className='break-words'>
                  {row.channel_name} (#{row.channel_id})
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Operation')}</dt>
                <dd className='break-all'>{row.operation || '-'}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Client model aliases')}
                </dt>
                <dd className='break-all'>{row.models.join(', ') || '-'}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Upstream model')}</dt>
                <dd className='break-all'>{row.upstream_model || '-'}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Effective state')}
                </dt>
                <dd>
                  <HealthStateBadge state={row.state} />
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Ready / total credentials')}
                </dt>
                <dd>
                  {formatNumber(row.ready_credentials)} /{' '}
                  {formatNumber(row.total_credentials)}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Snapshot time')}</dt>
                <dd>{formatTimestampToDate(props.generatedAt)}</dd>
              </div>
            </dl>
            {row.resources.map((resource) => (
              <ResourceCard
                key={`${resource.scope}-${resource.credential_index}`}
                resource={resource}
              />
            ))}
          </>
        )}
        <DialogFooter>
          <DialogClose render={<Button variant='outline' />}>
            {t('Close')}
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
