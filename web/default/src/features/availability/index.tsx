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
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { TooltipProvider } from '@/components/ui/tooltip'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { GroupGrid } from './components/group-grid'
import { useAvailability } from './hooks/use-availability'
import { availabilityBands, formatObservedTime } from './lib/presentation'
import type { AvailabilityRange } from './types'

export function AvailabilityPage() {
  const { t, i18n } = useTranslation()
  const [range, setRange] = useState<AvailabilityRange>('24h')
  const query = useAvailability(range)
  const report = query.report
  const previousRange = report !== undefined && report.range !== range
  const ranges: { value: AvailabilityRange; label: string }[] = [
    { value: '1h', label: t('1 hour') },
    { value: '24h', label: t('24 hours') },
    { value: '7d', label: t('7 days') },
    { value: '30d', label: t('30 days') },
  ]
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Group availability')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <TooltipProvider>
          <div className='flex min-w-0 flex-col gap-4'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Passive observations from real requests only. No active health checks are sent.'
              )}
            </p>
            <div className='flex flex-wrap items-center justify-between gap-3'>
              <ToggleGroup
                variant='outline'
                value={[range]}
                aria-label={t('Time range')}
                onValueChange={(values) => {
                  const next = ranges.find(
                    (option) => option.value === values[0]
                  )
                  if (next) setRange(next.value)
                }}
              >
                {ranges.map((option) => (
                  <ToggleGroupItem key={option.value} value={option.value}>
                    {option.label}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <Button
                variant='outline'
                disabled={query.isFetching}
                onClick={() => void query.refetch()}
              >
                {query.isFetching ? t('Refreshing...') : t('Refresh')}
              </Button>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Refreshes every 60 seconds while this page is visible. Times use your local time zone.'
              )}
            </p>
            {report && (
              <div
                className='text-muted-foreground flex flex-col gap-1 text-xs'
                aria-live='polite'
              >
                <p>
                  {t('Statistics updated')}:{' '}
                  {formatObservedTime(report.generated_at, i18n.language)}
                </p>
                <p>
                  {t('Displayed window')}:{' '}
                  {formatObservedTime(report.window_start, i18n.language)} –{' '}
                  {formatObservedTime(report.window_end, i18n.language)}
                </p>
                <p>
                  {t('Recent status window')}:{' '}
                  {formatObservedTime(report.current_start, i18n.language)} –{' '}
                  {formatObservedTime(report.window_end, i18n.language)}
                </p>
                <p>
                  {t('Observation coverage starts')}:{' '}
                  {formatObservedTime(report.coverage_start, i18n.language)}
                </p>
              </div>
            )}
            {report && (query.isError || previousRange) && (
              <Alert variant={query.isError ? 'destructive' : 'default'}>
                <AlertTitle>
                  {query.isError
                    ? t('Refresh failed — showing older observations')
                    : t('Updating range — showing previous observations')}
                </AlertTitle>
                <AlertDescription>
                  {t(
                    'These observations are not a fresh health assessment. The displayed window and update time belong to the retained report.'
                  )}
                </AlertDescription>
              </Alert>
            )}
            <div className='flex flex-col gap-2'>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'History colors show request success rates, not time-based uptime. Incomplete coverage is not the same as no traffic. Select a block for details.'
                )}
              </p>
              <div
                className='flex flex-wrap gap-3'
                aria-label={t('Status legend')}
              >
                {Object.entries(availabilityBands).map(([key, state]) => (
                  <StatusBadge
                    key={key}
                    variant={state.variant}
                    dotClassName={state.dotClassName}
                    copyable={false}
                    label={`${t(state.labelKey)} ${state.rangeLabel}`.trim()}
                  />
                ))}
              </div>
            </div>
            {!report && query.isError && (
              <ErrorState
                title={t('Unable to load availability')}
                description={t(
                  'Please retry to load real request observations.'
                )}
                onRetry={() => void query.refetch()}
              />
            )}
            {!report && !query.isError && <LoadingState />}
            {report && report.groups.length === 0 && (
              <EmptyState
                title={t('No visible groups')}
                description={t(
                  'Availability will appear when a display group is available.'
                )}
              />
            )}
            {report && <GroupGrid groups={report.groups} />}
          </div>
        </TooltipProvider>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
