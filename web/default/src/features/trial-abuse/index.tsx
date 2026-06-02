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
import { useMemo, useState, type JSX, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, InfoIcon, RefreshCw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getTrialAbuseSummary } from './api'
import {
  buildTrialAbuseSummaryParams,
  createDefaultTrialAbuseDraftFilters,
  dateTimeInputToUnixSeconds,
  trialAbuseRiskReasonI18nKey,
  trialAbuseWarningReasonI18nKey,
  unixSecondsToDateTimeInput,
  updateTrialAbuseDraftFilter,
  validateTrialAbuseDraftFilters,
  type TrialAbuseDraftFilters,
} from './lib/filters'
import type {
  TrialAbuseIPCluster,
  TrialAbuseInviterCluster,
  TrialAbuseRiskLevel,
  TrialAbuseRiskUser,
  TrialAbuseSelfInviteChain,
  TrialAbuseSummaryParams,
  TrialAbuseSummaryResponse,
  TrialAbuseWarningReason,
} from './types'

export type TrialAbusePageState = {
  draft: TrialAbuseDraftFilters
  submittedCriteria: TrialAbuseSummaryParams | null
}

export type TrialAbusePageAction =
  | { type: 'draft'; draft: TrialAbuseDraftFilters }
  | { type: 'submit' }
  | { type: 'reset'; draft: TrialAbuseDraftFilters }

export function reduceTrialAbusePageState(
  state: TrialAbusePageState,
  action: TrialAbusePageAction
): TrialAbusePageState {
  switch (action.type) {
    case 'draft':
      return { ...state, draft: action.draft }
    case 'submit':
      return {
        draft: state.draft,
        submittedCriteria: buildTrialAbuseSummaryParams(state.draft),
      }
    case 'reset':
      return { ...state, draft: action.draft }
  }
}

export function trialAbuseSummaryQueryEnabled(
  submittedCriteria: TrialAbuseSummaryParams | null
): boolean {
  return submittedCriteria != null
}

export function trialAbuseSummaryQueryKey(
  submittedCriteria: TrialAbuseSummaryParams | null
): readonly [string, string, TrialAbuseSummaryParams | null] {
  return ['trial-abuse', 'summary', submittedCriteria]
}

export function trialAbuseSummaryHasRisk(
  summary: TrialAbuseSummaryResponse | undefined
): boolean {
  if (summary == null) return false
  return (
    summary.risk_users.length > 0 ||
    summary.inviter_clusters.some(
      (cluster) => cluster.risk_participation === 'risk'
    ) ||
    summary.self_invite_chains.some((chain) => chain.registration_ip_available)
  )
}

export function TrialAbusePage(): JSX.Element {
  const { t } = useTranslation()
  const [state, setState] = useState<TrialAbusePageState>(() => ({
    draft: createDefaultTrialAbuseDraftFilters(),
    submittedCriteria: null,
  }))
  const validation = useMemo(
    () => validateTrialAbuseDraftFilters(state.draft),
    [state.draft]
  )

  const summaryQuery = useQuery({
    queryKey: trialAbuseSummaryQueryKey(state.submittedCriteria),
    queryFn: () => {
      if (state.submittedCriteria == null) {
        throw new Error('trial abuse summary query is disabled without criteria')
      }
      return getTrialAbuseSummary(state.submittedCriteria)
    },
    enabled: trialAbuseSummaryQueryEnabled(state.submittedCriteria),
    refetchInterval: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  const summary =
    summaryQuery.data?.success === true ? summaryQuery.data.data : undefined
  const hasResponseError = summaryQuery.data?.success === false
  const hasAnyRisk = trialAbuseSummaryHasRisk(summary)

  function submitCurrentDraft() {
    if (!validation.valid) return
    setState((current) => reduceTrialAbusePageState(current, { type: 'submit' }))
  }

  function resetDraft() {
    setState((current) =>
      reduceTrialAbusePageState(current, {
        type: 'reset',
        draft: createDefaultTrialAbuseDraftFilters(),
      })
    )
  }

  function refreshCurrentResult() {
    if (state.submittedCriteria != null) {
      void summaryQuery.refetch()
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('trialAbuse.title')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('trialAbuse.description')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <Alert>
            <InfoIcon className='size-4' />
            <AlertTitle>{t('trialAbuse.readOnlyNotice')}</AlertTitle>
            <AlertDescription>{t('trialAbuse.manualQueryNotice')}</AlertDescription>
          </Alert>

          <TrialAbuseFilterCard
            draft={state.draft}
            errors={validation.errors}
            querying={summaryQuery.isFetching}
            canRefresh={state.submittedCriteria != null}
            onDraftChange={(next) =>
              setState((current) =>
                reduceTrialAbusePageState(current, { type: 'draft', draft: next })
              )
            }
            onSubmit={submitCurrentDraft}
            onReset={resetDraft}
            onRefresh={refreshCurrentResult}
          />

          {state.submittedCriteria == null ? (
            <Alert>
              <Search className='size-4' />
              <AlertTitle>{t('trialAbuse.manualQueryNotice')}</AlertTitle>
              <AlertDescription>{t('trialAbuse.filter.description')}</AlertDescription>
            </Alert>
          ) : null}

          {summaryQuery.isFetching && summary == null ? (
            <Card>
              <CardContent className='text-muted-foreground py-8 text-sm'>
                {t('trialAbuse.loading')}
              </CardContent>
            </Card>
          ) : null}

          {summaryQuery.isError || hasResponseError ? (
            <ErrorState
              title={t('trialAbuse.error.title')}
              description={t('trialAbuse.error.description')}
              onRetry={refreshCurrentResult}
            />
          ) : null}

          {summary != null ? <TrialAbuseWarnings summary={summary} /> : null}
          {summary != null ? <TrialAbuseOverviewCards summary={summary} /> : null}
          {summary != null ? <TrialAbuseUsageCard summary={summary} /> : null}
          {summary != null ? <TrialAbuseClusterCards summary={summary} /> : null}
          {summary != null ? <TrialAbuseRiskUsersCard summary={summary} /> : null}
          {summary != null && !hasAnyRisk ? <TrialAbuseEmptyState /> : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function TrialAbuseFilterCard(props: {
  draft: TrialAbuseDraftFilters
  errors: string[]
  querying: boolean
  canRefresh: boolean
  onDraftChange: (next: TrialAbuseDraftFilters) => void
  onSubmit: () => void
  onReset: () => void
  onRefresh: () => void
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('trialAbuse.filter.title')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
          <DateTimeField
            id='trial-abuse-trial-end-start'
            label={t('trialAbuse.field.trialEndStart')}
            value={props.draft.trialEndStart}
            onChange={(value) =>
              props.onDraftChange(
                updateTrialAbuseDraftFilter(props.draft, 'trialEndStart', value)
              )
            }
          />
          <DateTimeField
            id='trial-abuse-trial-end-end'
            label={t('trialAbuse.field.trialEndEnd')}
            value={props.draft.trialEndEnd}
            onChange={(value) =>
              props.onDraftChange(
                updateTrialAbuseDraftFilter(props.draft, 'trialEndEnd', value)
              )
            }
          />
          <DateTimeField
            id='trial-abuse-registered-start'
            label={t('trialAbuse.field.registeredStart')}
            value={props.draft.registeredStart}
            onChange={(value) =>
              props.onDraftChange(
                updateTrialAbuseDraftFilter(props.draft, 'registeredStart', value)
              )
            }
          />
          <DateTimeField
            id='trial-abuse-registered-end'
            label={t('trialAbuse.field.registeredEnd')}
            value={props.draft.registeredEnd}
            onChange={(value) =>
              props.onDraftChange(
                updateTrialAbuseDraftFilter(props.draft, 'registeredEnd', value)
              )
            }
          />
          <NumberField
            id='trial-abuse-min-consume-count'
            label={t('trialAbuse.field.minConsumeCount')}
            min={1}
            max={100000}
            value={props.draft.minConsumeCount}
            onChange={(value) =>
              props.onDraftChange(
                updateTrialAbuseDraftFilter(props.draft, 'minConsumeCount', value)
              )
            }
          />
          <NumberField
            id='trial-abuse-min-cluster-size'
            label={t('trialAbuse.field.minClusterSize')}
            min={2}
            max={100}
            value={props.draft.minClusterSize}
            onChange={(value) =>
              props.onDraftChange(
                updateTrialAbuseDraftFilter(props.draft, 'minClusterSize', value)
              )
            }
          />
        </div>

        {props.errors.length > 0 ? (
          <div className='text-destructive space-y-1 text-sm'>
            {props.errors.map((error) => (
              <div key={error}>
                {error === 'trialAbuse.validation.trialEndRangeTooLarge'
                  ? t(error, { maxDays: 90 })
                  : t(error)}
              </div>
            ))}
          </div>
        ) : null}

        <div className='flex flex-wrap gap-2'>
          <Button onClick={props.onSubmit} disabled={props.querying}>
            {props.querying ? t('trialAbuse.querying') : t('trialAbuse.query')}
          </Button>
          <Button variant='outline' onClick={props.onReset}>
            {t('trialAbuse.reset')}
          </Button>
          <Button
            variant='outline'
            onClick={props.onRefresh}
            disabled={!props.canRefresh || props.querying}
          >
            <RefreshCw className='size-4' />
            {t('trialAbuse.refreshCurrentResult')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function DateTimeField(props: {
  id: string
  label: string
  value: number
  onChange: (value: number) => void
}): JSX.Element {
  return (
    <div className='grid gap-2'>
      <Label htmlFor={props.id}>{props.label}</Label>
      <Input
        id={props.id}
        type='datetime-local'
        value={unixSecondsToDateTimeInput(props.value)}
        onChange={(event) =>
          props.onChange(dateTimeInputToUnixSeconds(event.target.value))
        }
      />
    </div>
  )
}

function NumberField(props: {
  id: string
  label: string
  min: number
  max: number
  value: number
  onChange: (value: number) => void
}): JSX.Element {
  return (
    <div className='grid gap-2'>
      <Label htmlFor={props.id}>{props.label}</Label>
      <Input
        id={props.id}
        type='number'
        min={props.min}
        max={props.max}
        value={props.value}
        onChange={(event) => props.onChange(Number(event.target.value))}
      />
    </div>
  )
}

function TrialAbuseWarnings(props: {
  summary: TrialAbuseSummaryResponse
}): JSX.Element | null {
  const { t } = useTranslation()
  if (props.summary.warnings.length === 0) return null

  return (
    <div className='space-y-2'>
      {props.summary.warnings.map((warning) => (
        <Alert key={`${warning.section}:${warning.reason}`}>
          <AlertTriangle className='size-4' />
          <AlertTitle>{t(trialAbuseWarningReasonI18nKey(warning.reason))}</AlertTitle>
          <AlertDescription>
            {t(`trialAbuse.section.${warning.section}`)}
          </AlertDescription>
        </Alert>
      ))}
    </div>
  )
}

function TrialAbuseOverviewCards(props: {
  summary: TrialAbuseSummaryResponse
}): JSX.Element {
  const { t } = useTranslation()
  const metrics = [
    ['trialAbuse.field.totalTrialUsers', props.summary.overview.total_trial_users],
    ['trialAbuse.field.activeTrialUsers', props.summary.overview.active_trial_users],
    ['trialAbuse.field.expiredTrialUsers', props.summary.overview.expired_trial_users],
    [
      'trialAbuse.field.expiredUnpaidTrialUsers',
      props.summary.overview.expired_unpaid_trial_users,
    ],
    [
      'trialAbuse.field.highUsageCandidateUsers',
      props.summary.overview.high_usage_candidate_users,
    ],
    ['trialAbuse.field.riskUserCount', props.summary.overview.risk_user_count],
    ['trialAbuse.field.highRiskUserCount', props.summary.overview.high_risk_user_count],
    [
      'trialAbuse.field.mediumRiskUserCount',
      props.summary.overview.medium_risk_user_count,
    ],
    ['trialAbuse.field.lowRiskUserCount', props.summary.overview.low_risk_user_count],
    [
      'trialAbuse.field.managedInviterClusterCount',
      props.summary.overview.managed_inviter_cluster_count,
    ],
  ] as const
  return (
    <Card>
      <CardHeader>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <CardTitle>{t('trialAbuse.overview.title')}</CardTitle>
          <span className='text-muted-foreground text-xs'>
            {t('trialAbuse.generatedAt', {
              time: formatUnixSeconds(props.summary.generated_at),
            })}
          </span>
        </div>
      </CardHeader>
      <CardContent>
        <PartialBadge partial={props.summary.overview} />
        <div className='mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
          {metrics.map((metric) => (
            <MetricCard key={metric[0]} label={t(metric[0])} value={metric[1]} />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

function TrialAbuseUsageCard(props: {
  summary: TrialAbuseSummaryResponse
}): JSX.Element {
  const { t } = useTranslation()
  const usage = props.summary.usage_distribution
  const rows = [
    ['trialAbuse.field.sampleSize', usage.sample_size],
    ['trialAbuse.field.zeroUsageCount', usage.zero_usage_count],
    ['trialAbuse.field.aboveThresholdCount', usage.above_threshold_count],
    ['trialAbuse.field.p50', usage.p50],
    ['trialAbuse.field.p75', usage.p75],
    ['trialAbuse.field.p90', usage.p90],
    ['trialAbuse.field.p95', usage.p95],
    ['trialAbuse.field.p99', usage.p99],
  ] as const

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('trialAbuse.usageDistribution.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <PartialBadge partial={usage} />
        <div className='mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
          {rows.map((row) => (
            <MetricCard key={row[0]} label={t(row[0])} value={row[1]} />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

function TrialAbuseClusterCards(props: {
  summary: TrialAbuseSummaryResponse
}): JSX.Element {
  return (
    <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
      <IPClustersCard clusters={props.summary.ip_clusters} />
      <InviterClustersCard clusters={props.summary.inviter_clusters} />
      <SelfInviteChainsCard chains={props.summary.self_invite_chains} />
    </div>
  )
}

function IPClustersCard(props: {
  clusters: TrialAbuseIPCluster[]
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('trialAbuse.field.ipClusters')}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className='text-muted-foreground mb-3 text-sm'>
          {t('trialAbuse.ipUnavailableNotice')}
        </p>
        <CompactTable emptyLabel={t('trialAbuse.empty.title')}>
          <TableHeader>
            <TableRow>
              <TableHead>{t('trialAbuse.field.observedIp')}</TableHead>
              <TableHead>{t('trialAbuse.field.candidateCount')}</TableHead>
              <TableHead>{t('trialAbuse.field.totalConsumeCount')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.clusters.map((cluster) => (
              <TableRow key={`${cluster.observed_ip}:${cluster.ip_source}`}>
                <TableCell>{cluster.observed_ip || '-'}</TableCell>
                <TableCell>{cluster.candidate_count}</TableCell>
                <TableCell>{cluster.total_consume_count}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </CompactTable>
      </CardContent>
    </Card>
  )
}

function InviterClustersCard(props: {
  clusters: TrialAbuseInviterCluster[]
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('trialAbuse.field.inviterClusters')}</CardTitle>
      </CardHeader>
      <CardContent>
        <CompactTable emptyLabel={t('trialAbuse.empty.title')}>
          <TableHeader>
            <TableRow>
              <TableHead>{t('trialAbuse.field.inviter')}</TableHead>
              <TableHead>{t('trialAbuse.field.candidateCount')}</TableHead>
              <TableHead>{t('trialAbuse.field.paidConversionRate')}</TableHead>
              <TableHead>{t('trialAbuse.field.managed')}</TableHead>
              <TableHead>{t('trialAbuse.field.riskParticipation')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.clusters.map((cluster) => (
              <TableRow key={cluster.inviter_id}>
                <TableCell>
                  {cluster.inviter_username || `#${cluster.inviter_id}`}
                </TableCell>
                <TableCell>{cluster.candidate_count}</TableCell>
                <TableCell>{formatPercent(cluster.paid_conversion_rate)}</TableCell>
                <TableCell>{cluster.managed ? '✓' : '-'}</TableCell>
                <TableCell>
                  {t(
                    cluster.risk_participation === 'risk'
                      ? 'trialAbuse.riskParticipation.risk'
                      : 'trialAbuse.riskParticipation.displayOnly'
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </CompactTable>
      </CardContent>
    </Card>
  )
}

function SelfInviteChainsCard(props: {
  chains: TrialAbuseSelfInviteChain[]
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <Card className='xl:col-span-2'>
      <CardHeader>
        <CardTitle>{t('trialAbuse.field.selfInviteChains')}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className='text-muted-foreground mb-3 text-sm'>
          {t('trialAbuse.ipUnavailableNotice')}
        </p>
        <CompactTable emptyLabel={t('trialAbuse.empty.title')}>
          <TableHeader>
            <TableRow>
              <TableHead>{t('trialAbuse.field.chainId')}</TableHead>
              <TableHead>{t('trialAbuse.field.registrationIp')}</TableHead>
              <TableHead>{t('trialAbuse.field.candidateCount')}</TableHead>
              <TableHead>{t('trialAbuse.field.totalConsumeCount')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.chains.map((chain) => (
              <TableRow key={chain.chain_id}>
                <TableCell>{chain.chain_id}</TableCell>
                <TableCell>{chain.registration_ip || '-'}</TableCell>
                <TableCell>{chain.candidate_count}</TableCell>
                <TableCell>{chain.total_consume_count}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </CompactTable>
      </CardContent>
    </Card>
  )
}

function TrialAbuseRiskUsersCard(props: {
  summary: TrialAbuseSummaryResponse
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('trialAbuse.field.riskUsers')}</CardTitle>
      </CardHeader>
      <CardContent>
        <PartialBadge partial={props.summary.risk_counts} />
        <CompactTable emptyLabel={t('trialAbuse.empty.title')}>
          <TableHeader>
            <TableRow>
              <TableHead>{t('trialAbuse.field.userId')}</TableHead>
              <TableHead>{t('trialAbuse.field.username')}</TableHead>
              <TableHead>{t('trialAbuse.field.inviter')}</TableHead>
              <TableHead>{t('trialAbuse.field.trialSource')}</TableHead>
              <TableHead>{t('trialAbuse.field.trialEnd')}</TableHead>
              <TableHead>{t('trialAbuse.field.consumeCount')}</TableHead>
              <TableHead>{t('trialAbuse.field.observedIp')}</TableHead>
              <TableHead>{t('trialAbuse.field.ipSource')}</TableHead>
              <TableHead>{t('trialAbuse.field.riskLevel')}</TableHead>
              <TableHead>{t('trialAbuse.field.riskReasons')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.summary.risk_users.map((user) => (
              <TrialAbuseRiskUserRow key={user.user_id} user={user} />
            ))}
          </TableBody>
        </CompactTable>
      </CardContent>
    </Card>
  )
}

function TrialAbuseRiskUserRow(props: {
  user: TrialAbuseRiskUser
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <TableRow>
      <TableCell>#{props.user.user_id}</TableCell>
      <TableCell>{props.user.username}</TableCell>
      <TableCell>
        {props.user.inviter_username ||
          (props.user.inviter_id > 0 ? `#${props.user.inviter_id}` : '-')}
      </TableCell>
      <TableCell>{props.user.trial_source || '-'}</TableCell>
      <TableCell>{formatUnixSeconds(props.user.trial_end_time)}</TableCell>
      <TableCell>{props.user.consume_count}</TableCell>
      <TableCell>{props.user.observed_ip || '-'}</TableCell>
      <TableCell>{props.user.ip_source || '-'}</TableCell>
      <TableCell>
        <RiskLevelBadge level={props.user.risk_level} />
      </TableCell>
      <TableCell>
        <div className='flex max-w-96 flex-wrap gap-1'>
          {props.user.risk_reasons.map((reason) => (
            <Badge key={reason} variant='outline'>
              {t(trialAbuseRiskReasonI18nKey(reason))}
            </Badge>
          ))}
        </div>
      </TableCell>
    </TableRow>
  )
}

function RiskLevelBadge(props: { level: TrialAbuseRiskLevel }): JSX.Element {
  const { t } = useTranslation()
  const variant = props.level === 'high' ? 'destructive' : 'secondary'
  return <Badge variant={variant}>{t(`trialAbuse.riskLevel.${props.level}`)}</Badge>
}

function MetricCard(props: { label: string; value: number | string }): JSX.Element {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 text-2xl font-semibold'>{props.value}</div>
    </div>
  )
}

function PartialBadge(props: {
  partial: { partial: boolean; partial_reasons: TrialAbuseWarningReason[] }
}): JSX.Element | null {
  const { t } = useTranslation()
  if (!props.partial.partial) return null
  return (
    <div className='mb-2 flex flex-wrap items-center gap-2'>
      <Badge variant='outline'>{t('trialAbuse.partialResult')}</Badge>
      {props.partial.partial_reasons.map((reason) => (
        <Badge key={reason} variant='secondary'>
          {t(trialAbuseWarningReasonI18nKey(reason))}
        </Badge>
      ))}
    </div>
  )
}

function CompactTable(props: {
  children: ReactNode
  emptyLabel: string
}): JSX.Element {
  return (
    <div className='overflow-x-auto rounded-lg border'>
      <Table>{props.children}</Table>
      <div className='text-muted-foreground hidden p-3 text-sm empty:block'>
        {props.emptyLabel}
      </div>
    </div>
  )
}

function TrialAbuseEmptyState(): JSX.Element {
  const { t } = useTranslation()
  return (
    <Empty>
      <EmptyHeader>
        <EmptyTitle>{t('trialAbuse.empty.title')}</EmptyTitle>
        <EmptyDescription>{t('trialAbuse.empty.description')}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

function formatUnixSeconds(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '-'
  return new Date(value * 1000).toLocaleString()
}

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(1)}%`
}
