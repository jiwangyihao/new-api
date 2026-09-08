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
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { StatusBadge } from '@/components/status-badge'
import {
  availabilityStates,
  observedState,
  formatLatency,
  formatObservedTime,
  formatRate,
} from '../lib/presentation'
import type { AvailabilityMetric } from '../types'

export function MetricStatus(props: { metric: AvailabilityMetric }) {
  const { t } = useTranslation()
  const state = availabilityStates[observedState(props.metric)]
  return (
    <StatusBadge
      variant={state.variant}
      copyable={false}
      label={t(state.labelKey)}
    />
  )
}
export function ObservationNotes(props: { metric: AvailabilityMetric }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap gap-2'>
      {props.metric.coverage === 'incomplete' && (
        <Badge variant='outline'>{t('Incomplete coverage')}</Badge>
      )}
      {props.metric.low_sample && (
        <Badge variant='secondary'>{t('Low sample')}</Badge>
      )}
      {props.metric.has_failures && (
        <StatusBadge
          variant='warning'
          copyable={false}
          label={t('Failures observed')}
        />
      )}
    </div>
  )
}
export function MetricDetails(props: {
  current: AvailabilityMetric
  summary: AvailabilityMetric
}) {
  const { t, i18n } = useTranslation()
  return (
    <div className='flex flex-col gap-3'>
      <section
        aria-label={t('Recent status (15 minutes)')}
        className='flex flex-col gap-2'
      >
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h3 className='text-sm font-medium'>
            {t('Recent status (15 minutes)')}
          </h3>
          <MetricStatus metric={props.current} />
        </div>
        <ObservationNotes metric={props.current} />
      </section>
      <Separator />
      <section aria-label={t('Selected range')} className='flex flex-col gap-2'>
        <h3 className='text-muted-foreground text-sm font-medium'>
          {t('Selected range')}
        </h3>
        <dl className='grid grid-cols-2 gap-3 text-sm'>
          <div>
            <dt className='text-muted-foreground'>
              {t('Service success rate')}
            </dt>
            <dd className='font-medium tabular-nums'>
              {formatRate(props.summary.success_rate, i18n.language)}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>
              {t('Median first response')}
            </dt>
            <dd className='font-medium tabular-nums'>
              {formatLatency(props.summary.first_response_ms, i18n.language)}
            </dd>
          </div>
          <div className='col-span-2'>
            <dt className='text-muted-foreground'>{t('Last observation')}</dt>
            <dd className='break-words'>
              {formatObservedTime(
                props.summary.last_observed_at,
                i18n.language
              )}
            </dd>
          </div>
        </dl>
        <ObservationNotes metric={props.summary} />
      </section>
    </div>
  )
}
