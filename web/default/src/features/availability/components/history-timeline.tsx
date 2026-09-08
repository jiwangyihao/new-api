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
import { cn } from '@/lib/utils'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverDescription,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { EmptyState } from '@/components/empty-state'
import {
  availabilityBands,
  observedBand,
  formatLatency,
  formatObservedTime,
  formatRate,
} from '../lib/presentation'
import type { AvailabilityBucket } from '../types'

function HistoryBucket(props: { bucket: AvailabilityBucket }) {
  const { t, i18n } = useTranslation()
  const bucket = props.bucket
  const state = availabilityBands[observedBand(bucket)]
  const interval = `${formatObservedTime(bucket.start, i18n.language)} – ${formatObservedTime(bucket.end, i18n.language)}`
  const detail = [
    `${t(state.labelKey)} ${state.rangeLabel}`.trim(),
    `${t('Service success rate')}: ${formatRate(bucket.success_rate, i18n.language)}`,
    `${t('Median first response')}: ${formatLatency(bucket.first_response_ms, i18n.language)}`,
    bucket.coverage === 'complete'
      ? t('Complete coverage')
      : t('Incomplete coverage'),
    bucket.low_sample ? t('Low sample') : '',
    bucket.has_failures ? t('Failures observed') : '',
  ]
    .filter(Boolean)
    .join(' · ')
  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger
          render={<PopoverTrigger render={<button type='button' />} />}
          aria-label={`${interval} · ${detail}`}
          className='focus-visible:ring-ring min-w-0 flex-1 rounded-sm py-2 outline-none focus-visible:ring-2'
        >
          <span
            aria-hidden='true'
            className={cn('block h-5 rounded-sm', state.dotClassName)}
          />
        </TooltipTrigger>
        <TooltipContent>
          <span>
            {interval}
            <br />
            {detail}
          </span>
        </TooltipContent>
      </Tooltip>
      <PopoverContent>
        <PopoverTitle>{interval}</PopoverTitle>
        <PopoverDescription>{detail}</PopoverDescription>
      </PopoverContent>
    </Popover>
  )
}
export function HistoryTimeline(props: { buckets: AvailabilityBucket[] }) {
  const { t, i18n } = useTranslation()
  if (props.buckets.length === 0)
    return (
      <EmptyState className='min-h-24' title={t('No valid observations')} />
    )
  return (
    <section aria-label={t('Service success history')} className='min-w-0'>
      <h3 className='text-muted-foreground text-xs'>
        {t('Service success history')}
      </h3>
      <div className='flex min-w-0 gap-0.5'>
        {props.buckets.map((bucket) => (
          <HistoryBucket key={bucket.start} bucket={bucket} />
        ))}
      </div>
      <div className='text-muted-foreground flex flex-wrap justify-between gap-2 text-xs'>
        <span>{formatObservedTime(props.buckets[0].start, i18n.language)}</span>
        <span>
          {formatObservedTime(
            props.buckets[props.buckets.length - 1].end,
            i18n.language
          )}
        </span>
      </div>
    </section>
  )
}
