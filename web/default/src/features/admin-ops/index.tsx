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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { ErrorState } from '@/components/error-state'
import { getAdminOpsConcurrency, getAdminOpsSnapshot } from './api'
import { AdminOpsHeader } from './components/admin-ops-header'
import { ChannelHealthCard } from './components/channel-health-card'
import { ConcurrencyQueueCard } from './components/concurrency-queue-card'
import { HealthSummaryCards } from './components/health-summary-cards'
import { PerformanceModelsCard } from './components/performance-models-card'
import { RealtimeTrafficCard } from './components/realtime-traffic-card'
import { RecentErrorsCard } from './components/recent-errors-card'

const SNAPSHOT_REFETCH_MS = 30_000
const CONCURRENCY_REFETCH_MS = 5_000

export function AdminOpsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [autoRefresh, setAutoRefresh] = useState(true)

  const snapshotQuery = useQuery({
    queryKey: ['admin-ops', 'snapshot', 300, 5],
    queryFn: () => getAdminOpsSnapshot({ window_seconds: 300, top: 5 }),
    refetchInterval: autoRefresh ? SNAPSHOT_REFETCH_MS : false,
    refetchIntervalInBackground: false,
  })

  const concurrencyQuery = useQuery({
    queryKey: ['admin-ops', 'concurrency', 20, true, 1],
    queryFn: () =>
      getAdminOpsConcurrency({
        limit: 20,
        include_users: true,
        min_active_or_queued: 1,
      }),
    refetchInterval: autoRefresh ? CONCURRENCY_REFETCH_MS : false,
    refetchIntervalInBackground: false,
  })

  const snapshot =
    snapshotQuery.data?.success === true ? snapshotQuery.data.data : undefined
  const concurrency =
    concurrencyQuery.data?.success === true
      ? concurrencyQuery.data.data
      : snapshot?.concurrency
  const hasError =
    snapshotQuery.isError ||
    concurrencyQuery.isError ||
    snapshotQuery.data?.success === false ||
    concurrencyQuery.data?.success === false
  const isRefreshing = snapshotQuery.isFetching || concurrencyQuery.isFetching
  const generatedAt = useMemo(
    () => Math.max(snapshot?.generated_at ?? 0, concurrency?.generated_at ?? 0),
    [snapshot?.generated_at, concurrency?.generated_at]
  )

  function refreshAll() {
    void queryClient.invalidateQueries({ queryKey: ['admin-ops'] })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('adminOps.title')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('adminOps.description')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <AdminOpsHeader
            health={snapshot?.health}
            generatedAt={generatedAt}
            refreshing={isRefreshing}
            autoRefresh={autoRefresh}
            onAutoRefreshChange={setAutoRefresh}
            onRefresh={refreshAll}
          />
          {hasError ? (
            <ErrorState
              title={t('adminOps.failedToLoad')}
              description={t('adminOps.failedToLoadDescription')}
              onRetry={refreshAll}
            />
          ) : null}
          <HealthSummaryCards
            snapshot={snapshot}
            loading={snapshotQuery.isLoading && !snapshot}
          />
          <ConcurrencyQueueCard
            concurrency={concurrency}
            loading={concurrencyQuery.isLoading && !concurrency}
          />
          <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
            <RealtimeTrafficCard
              snapshot={snapshot}
              loading={snapshotQuery.isLoading && !snapshot}
            />
            <ChannelHealthCard
              snapshot={snapshot}
              loading={snapshotQuery.isLoading && !snapshot}
            />
          </div>
          <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
            <PerformanceModelsCard
              snapshot={snapshot}
              loading={snapshotQuery.isLoading && !snapshot}
            />
            <RecentErrorsCard
              snapshot={snapshot}
              loading={snapshotQuery.isLoading && !snapshot}
            />
          </div>
          <Button variant='outline' className='sr-only' onClick={refreshAll}>
            {t('Refresh')}
          </Button>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
