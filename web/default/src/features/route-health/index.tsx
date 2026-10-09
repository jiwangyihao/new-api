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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { getRouteHealth } from './api'
import { ChannelHealthOverview } from './components/channel-health-overview'

export function RouteHealth() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['channel', 'route-health'],
    queryFn: ({ signal }) => getRouteHealth(signal),
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
    retry: false,
  })
  const report = query.data
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Route health overview')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          onClick={() => void query.refetch()}
          disabled={query.isFetching}
        >
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Channels first, problems first. Expand a channel to inspect its models and operations.'
            )}
          </p>
          {query.isError ? (
            <ErrorState
              title={t('Unable to load route health')}
              description={t(
                'The latest snapshot could not be loaded. Previous availability states are hidden until refresh succeeds.'
              )}
              onRetry={() => void query.refetch()}
            />
          ) : (
            <>
              {report && (
                <>
                  {report.degraded && (
                    <Alert variant='destructive'>
                      <AlertTitle>
                        {t('Degraded route health protection')}
                      </AlertTitle>
                      <AlertDescription>
                        {t(
                          'Redis is unavailable; this snapshot reflects protection on this instance, not complete cluster state.'
                        )}
                      </AlertDescription>
                    </Alert>
                  )}
                  {!report.enabled && (
                    <Alert>
                      <AlertTitle>{t('Health protection disabled')}</AlertTitle>
                      <AlertDescription>
                        {t(
                          'Health protection is disabled; rows remain configuration inventory and are not availability proof.'
                        )}
                      </AlertDescription>
                    </Alert>
                  )}
                  <div className='text-muted-foreground flex flex-wrap items-center gap-x-5 gap-y-2 text-xs'>
                    <span>
                      {t('Snapshot time')}:{' '}
                      {formatTimestampToDate(report.generated_at)}
                    </span>
                    <span>
                      {t(
                        'Refreshes every 15 seconds while this page is visible.'
                      )}
                    </span>
                    <Collapsible>
                      <CollapsibleTrigger className='hover:text-foreground underline underline-offset-4'>
                        {t('Protection settings')}
                      </CollapsibleTrigger>
                      <CollapsibleContent className='space-y-2 py-3'>
                        <p>
                          {t(
                            'Maximum attempts: {{attempts}}; additional retries: {{retries}}',
                            {
                              attempts: formatNumber(report.max_attempts),
                              retries: formatNumber(report.retry_times),
                            }
                          )}
                        </p>
                        <p>
                          {report.storage === 'memory'
                            ? t('Storage: memory (this instance only)')
                            : t('Storage: Redis')}
                        </p>
                        <p>
                          {t('Sample window: {{seconds}} seconds', {
                            seconds: formatNumber(report.sample_window_seconds),
                          })}{' '}
                          ·{' '}
                          {t('Failure window: {{seconds}} seconds', {
                            seconds: formatNumber(
                              report.failure_window_seconds
                            ),
                          })}
                        </p>
                      </CollapsibleContent>
                    </Collapsible>
                  </div>
                  <ChannelHealthOverview report={report} />
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Unobserved is not healthy. Counts represent models, not repeated operations or shared resource samples.'
                    )}
                  </p>
                </>
              )}
              {query.isPending && <LoadingState />}
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
