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
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { getRouteHealth } from './api'
import { RouteHealthTable } from './components/route-health-table'

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
              'Live protection snapshot by actual upstream model and operation; this is not historical SLA or an active connectivity test.'
            )}{' '}
            {t(
              'Unobserved does not mean tested successfully. Healthy is not a guarantee of future availability.'
            )}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t('Refreshes every 15 seconds while this page is visible.')}
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
                  <div className='flex flex-wrap gap-x-6 gap-y-2 rounded-lg border p-3 text-sm'>
                    <span>
                      {t('Snapshot time')}:{' '}
                      {formatTimestampToDate(report.generated_at)}
                    </span>
                    <span>
                      {t(
                        'Maximum attempts: {{attempts}}; additional retries: {{retries}}',
                        {
                          attempts: formatNumber(report.max_attempts),
                          retries: formatNumber(report.retry_times),
                        }
                      )}
                    </span>
                    <span>
                      {report.storage === 'memory'
                        ? t('Storage: memory (this instance only)')
                        : t('Storage: Redis')}
                    </span>
                    <span>
                      {t('Sample window: {{seconds}} seconds', {
                        seconds: formatNumber(report.sample_window_seconds),
                      })}
                    </span>
                    <span>
                      {t('Failure window: {{seconds}} seconds', {
                        seconds: formatNumber(report.failure_window_seconds),
                      })}
                    </span>
                  </div>
                </>
              )}
              <RouteHealthTable
                report={report}
                isLoading={query.isPending}
                isFetching={query.isFetching}
              />
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
