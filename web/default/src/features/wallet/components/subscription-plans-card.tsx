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
import { useState, useEffect, useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Crown, RefreshCw, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { FieldDescription, FieldLegend, FieldSet } from '@/components/ui/field'
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  StatusBadge,
  dotColorMap,
  textColorMap,
} from '@/components/status-badge'
import {
  getCreditBalanceLedger,
  getPublicPlans,
  getSelfSubscriptionFull,
  resetSubscriptionQuota,
  setActiveSubscription,
  updateSubscriptionBillingStrategy,
} from '@/features/subscriptions/api'
import { CreditBalanceLedger } from '@/features/subscriptions/components/credit-balance-ledger'
import { SubscriptionPurchaseDialog } from '@/features/subscriptions/components/dialogs/subscription-purchase-dialog'
import {
  formatConcurrencyLimit,
  formatPlanPrice,
  formatDuration,
  formatCreditLimit,
  formatFiniteCreditCount,
} from '@/features/subscriptions/lib'
import { subscriptionQueryKeys } from '@/features/subscriptions/query-keys'
import type {
  PlanRecord,
  SelfSubscriptionData,
  UserSubscriptionRecord,
  SubscriptionBillingStrategy,
  CreditBalanceLedgerFilters,
} from '@/features/subscriptions/types'
import {
  formatPlanChannelEquivalent,
  formatSubscriptionChannelEquivalent,
  getChannelEquivalentNotes,
  getSubscriptionDisplayLabel,
  getVisibleChannelEquivalents,
  shouldShowChannelEquivalents,
} from '../lib/subscription-display'
import type { PaymentMethod, TopupInfo } from '../types'

type TranslationFn = (key: string, options?: Record<string, unknown>) => string

interface SubscriptionPlansCardProps {
  topupInfo: TopupInfo | null
  accountBalance?: number
  onPurchaseSuccess?: () => Promise<void> | void
  onAvailabilityChange?: (available: boolean) => void
}

function getEpayMethods(payMethods: PaymentMethod[] = []): PaymentMethod[] {
  return payMethods.filter(
    (m) => m?.type && m.type !== 'stripe' && m.type !== 'creem'
  )
}

function getSubscriptionEndTime(
  subscription: UserSubscriptionRecord['subscription'] | null | undefined
): number {
  return Number(subscription?.effective_end_time || subscription?.end_time || 0)
}

function getRawSubscriptionSource(
  record: UserSubscriptionRecord | null | undefined
): string {
  const subscription = record?.subscription
  return (
    subscription?.grant_reason?.trim() || subscription?.source?.trim() || ''
  )
}

function isPaidLikeSubscription(
  record: UserSubscriptionRecord | null | undefined
): boolean {
  const subscription = record?.subscription
  const sourceLabel = subscription?.source_label?.trim()
  if (sourceLabel === 'paid') return true
  const source = getRawSubscriptionSource(record)
  if (source === 'order' || source === 'redemption') return true
  if (source !== 'admin') return false
  const plan = record?.plan
  return !!plan && plan.price_amount > 0 && !plan.is_trial && !plan.invite_trial
}

export function getSubscriptionSourceLabel(
  record: UserSubscriptionRecord | null | undefined,
  t: TranslationFn
): string {
  const subscription = record?.subscription
  const sourceLabel = subscription?.source_label?.trim()
  if (sourceLabel === 'paid') return t('Paid plan')
  if (sourceLabel === 'invitation_reward') return t('Invitation reward')
  if (sourceLabel === 'trial') return t('Trial')
  const source = getRawSubscriptionSource(record)
  if (
    source === 'order' ||
    source === 'redemption' ||
    isPaidLikeSubscription(record)
  ) {
    return t('Paid plan')
  }
  if (source === 'monthly_invite_entitlement') return t('Invitation reward')
  if (source === 'trial_code' || source === 'invite_trial') return t('Trial')
  return t('Unknown')
}

export function canResetSubscriptionQuotaFromRecord(
  record: UserSubscriptionRecord | null | undefined,
  now: number
): boolean {
  const subscription = record?.subscription
  if (!subscription || subscription.entitlement_type === 'credit_balance') {
    return false
  }
  const endTime = getSubscriptionEndTime(subscription)
  const isExpired = endTime < now
  const isCancelled = subscription.status === 'cancelled'
  const isActive = subscription.status === 'active' && !isExpired
  return (
    (subscription.can_reset_quota ?? isPaidLikeSubscription(record)) &&
    isActive &&
    !isCancelled
  )
}

function formatUsedCreditCount(value: number, t: TranslationFn): string {
  if (value <= 0) return `0 ${t('credits')}`
  return formatCreditLimit(value, t)
}

export function renderPlanChannelEquivalentLabels(
  plan: PlanRecord['plan'],
  t: TranslationFn,
  limit = 3
): string[] {
  const equivalents = plan.channel_credit_equivalents ?? []
  const legacyEquivalents = plan.channel_token_equivalents ?? []
  if (equivalents.length === 0) {
    if (!shouldShowChannelEquivalents(legacyEquivalents)) return []
    const visible = getVisibleChannelEquivalents(legacyEquivalents, limit)
    const labels = visible.items.map((item) =>
      formatPlanChannelEquivalent(item, t)
    )
    if (visible.hiddenCount > 0) {
      labels.push(t('+{{count}} more', { count: visible.hiddenCount }))
    }
    return labels
  }
  if (!shouldShowChannelEquivalents(equivalents)) return []

  const visible = getVisibleChannelEquivalents(equivalents, limit)
  const labels = visible.items.map((item) =>
    formatPlanChannelEquivalent(item, t)
  )
  if (visible.hiddenCount > 0) {
    labels.push(t('+{{count}} more', { count: visible.hiddenCount }))
  }

  return labels
}

export function renderPlanChannelEquivalentNotes(
  plan: PlanRecord['plan'],
  t: TranslationFn
): string[] {
  const equivalents = plan.channel_credit_equivalents ?? []
  const legacyEquivalents = plan.channel_token_equivalents ?? []
  const visibleEquivalents =
    equivalents.length > 0 ? equivalents : legacyEquivalents
  if (!shouldShowChannelEquivalents(visibleEquivalents)) return []
  return getChannelEquivalentNotes(visibleEquivalents, t)
}

export function renderSubscriptionChannelEquivalentLabels(
  data: Pick<SelfSubscriptionData, 'summary'> | null,
  isCurrentActive: boolean,
  t: TranslationFn,
  limit = 3
): string[] {
  const equivalents = isCurrentActive
    ? (data?.summary?.channel_credit_equivalents ?? [])
    : []
  const legacyEquivalents = isCurrentActive
    ? (data?.summary?.channel_token_equivalents ?? [])
    : []
  if (equivalents.length === 0) {
    if (!shouldShowChannelEquivalents(legacyEquivalents)) return []
    const visible = getVisibleChannelEquivalents(legacyEquivalents, limit)
    const labels = visible.items.map((item) =>
      formatSubscriptionChannelEquivalent(item, t)
    )
    if (visible.hiddenCount > 0) {
      labels.push(t('+{{count}} more', { count: visible.hiddenCount }))
    }
    return labels
  }
  if (!shouldShowChannelEquivalents(equivalents)) return []

  const visible = getVisibleChannelEquivalents(equivalents, limit)
  const labels = visible.items.map((item) =>
    formatSubscriptionChannelEquivalent(item, t)
  )
  if (visible.hiddenCount > 0) {
    labels.push(t('+{{count}} more', { count: visible.hiddenCount }))
  }

  return labels
}

export function renderSubscriptionChannelEquivalentNotes(
  data: Pick<SelfSubscriptionData, 'summary'> | null,
  isCurrentActive: boolean,
  t: TranslationFn
): string[] {
  const equivalents = isCurrentActive
    ? (data?.summary?.channel_credit_equivalents ?? [])
    : []
  const legacyEquivalents = isCurrentActive
    ? (data?.summary?.channel_token_equivalents ?? [])
    : []
  const visibleEquivalents =
    equivalents.length > 0 ? equivalents : legacyEquivalents
  if (!shouldShowChannelEquivalents(visibleEquivalents)) return []
  return getChannelEquivalentNotes(visibleEquivalents, t)
}

export const billingStrategyOptions: Array<{
  value: SubscriptionBillingStrategy
  label: string
  description: string
}> = [
  {
    value: 'single_active',
    label: 'Single active subscription',
    description:
      'Uses only the active subscription. Credit shortage, model restrictions, concurrency limits, and request failures never switch benefits.',
  },
  {
    value: 'active_fallback',
    label: 'Active subscription fallback',
    description:
      'Uses the active subscription first. Only invalid, disabled, or Credit-insufficient benefits fall back to timed subscriptions by expiry, then Credit balance.',
  },
  {
    value: 'timed_first',
    label: 'Timed subscriptions first',
    description:
      'Ignores the active selection and tries timed subscriptions by expiry, then Credit balance. Each request still uses one benefit only.',
  },
]

export function SubscriptionBillingStrategyControl({
  data,
}: {
  data: SelfSubscriptionData
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const strategy = data.billing_strategy ?? 'single_active'
  const activeSubscriptionId = Number(data.active_subscription_id || 0)
  const recordsById = new Map(
    data.all_subscriptions.map((record) => [
      Number(record.subscription?.id || 0),
      record,
    ])
  )
  const describeSubscription = (subscriptionId: number) => {
    const record = recordsById.get(subscriptionId)
    return record?.plan?.title || `${t('Subscription')} #${subscriptionId}`
  }
  const updateMutation = useMutation({
    mutationFn: (next: SubscriptionBillingStrategy) =>
      updateSubscriptionBillingStrategy({ billing_strategy: next }),
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Request failed'))
        return
      }
      await queryClient.invalidateQueries({
        queryKey: subscriptionQueryKeys.selfSummary,
      })
      toast.success(t('Billing strategy updated'))
    },
    onError: () => {
      toast.error(t('Request failed'))
    },
  })

  const handleChange = (values: string[]) => {
    const next = values[0] as SubscriptionBillingStrategy | undefined
    if (!next || next === strategy || updateMutation.isPending) return
    updateMutation.mutate(next)
  }

  return (
    <FieldSet className='rounded-xl border p-3 sm:p-4'>
      <FieldLegend variant='label'>
        {t('Subscription billing strategy')}
      </FieldLegend>
      <FieldDescription>
        {t(
          'This account-level strategy applies to every API key and only affects requests that start after it changes.'
        )}
      </FieldDescription>
      <ToggleGroup
        value={[strategy]}
        onValueChange={handleChange}
        disabled={updateMutation.isPending}
        spacing={2}
        className='grid w-full grid-cols-1 gap-2 lg:grid-cols-3'
        aria-label={t('Subscription billing strategy')}
      >
        {billingStrategyOptions.map((option) => (
          <ToggleGroupItem
            key={option.value}
            value={option.value}
            variant='outline'
            className='h-auto min-h-20 w-full items-start justify-start p-3 text-left whitespace-normal'
            aria-label={t(option.label)}
          >
            <span>
              <span className='block font-medium'>{t(option.label)}</span>
              <span className='text-muted-foreground mt-1 block text-xs font-normal'>
                {t(option.description)}
              </span>
            </span>
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <div className='grid gap-3 text-xs sm:grid-cols-2'>
        <div>
          <div className='text-muted-foreground'>
            {t('Active subscription')}
          </div>
          <div className='mt-1 font-medium'>
            {activeSubscriptionId > 0
              ? describeSubscription(activeSubscriptionId)
              : t('No active subscription')}
          </div>
        </div>
        <div>
          <div className='text-muted-foreground'>
            {t('Current candidate order')}
          </div>
          {(data.billing_candidate_subscription_ids?.length || 0) > 0 ? (
            <ol className='mt-1 list-inside list-decimal space-y-1 font-medium'>
              {data.billing_candidate_subscription_ids?.map(
                (subscriptionId) => (
                  <li key={subscriptionId}>
                    {describeSubscription(subscriptionId)}
                  </li>
                )
              )}
            </ol>
          ) : (
            <div className='mt-1 font-medium'>
              {t('No billable candidates')}
            </div>
          )}
        </div>
      </div>
    </FieldSet>
  )
}

export function SubscriptionPlansCard({
  topupInfo,
  onAvailabilityChange,
  accountBalance,
  onPurchaseSuccess,
}: SubscriptionPlansCardProps) {
  const { t } = useTranslation()

  const [refreshing, setRefreshing] = useState(false)
  const [pendingActiveSubscriptionId, setPendingActiveSubscriptionId] =
    useState<number | null>(null)
  const [resetTarget, setResetTarget] = useState<UserSubscriptionRecord | null>(
    null
  )
  const [resettingQuotaId, setResettingQuotaId] = useState<number | null>(null)
  const [purchaseOpen, setPurchaseOpen] = useState(false)
  const [selectedPlan, setSelectedPlan] = useState<PlanRecord | null>(null)

  const enableStripe = !!topupInfo?.enable_stripe_topup
  const enableCreem = !!topupInfo?.enable_creem_topup
  const enableOnlineTopUp = !!topupInfo?.enable_online_topup
  const epayMethods = useMemo(
    () => getEpayMethods(topupInfo?.pay_methods),
    [topupInfo?.pay_methods]
  )

  const plansQuery = useQuery({
    queryKey: subscriptionQueryKeys.walletPlans,
    queryFn: async () => {
      try {
        return await getPublicPlans()
      } catch {
        return { success: false, data: [] }
      }
    },
  })

  const selfSubscriptionQuery = useQuery({
    queryKey: subscriptionQueryKeys.selfSummary,
    queryFn: getSelfSubscriptionFull,
  })

  const loadSelfCreditLedger = useMemo(
    () => (filters: CreditBalanceLedgerFilters) =>
      getCreditBalanceLedger(filters),
    []
  )

  const plans = plansQuery.data?.success ? (plansQuery.data.data ?? []) : []
  const selfSubscriptionData = selfSubscriptionQuery.data?.success
    ? (selfSubscriptionQuery.data.data ?? null)
    : null
  const activeSubscriptions = selfSubscriptionData?.subscriptions ?? []
  const allSubscriptions = selfSubscriptionData?.all_subscriptions ?? []
  const backendActiveSubscriptionId = Number(
    selfSubscriptionData?.active_subscription_id ||
      selfSubscriptionData?.summary?.active_subscription_id ||
      selfSubscriptionData?.summary?.subscription_id ||
      0
  )
  const activeSubscriptionId =
    backendActiveSubscriptionId > 0 ? backendActiveSubscriptionId : null
  const loading = plansQuery.isLoading || selfSubscriptionQuery.isLoading

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      await selfSubscriptionQuery.refetch()
    } finally {
      setRefreshing(false)
    }
  }

  const handleSetActiveSubscription = async (subscriptionId: number) => {
    setPendingActiveSubscriptionId(subscriptionId)
    try {
      const res = await setActiveSubscription({
        subscription_id: subscriptionId,
      })
      if (res.success) {
        toast.success(t('Active subscription updated'))
        await selfSubscriptionQuery.refetch()
        return
      }
      toast.error(res.message || t('Request failed'))
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setPendingActiveSubscriptionId(null)
    }
  }

  const handleConfirmResetQuota = async () => {
    const subscriptionId = resetTarget?.subscription?.id
    if (!subscriptionId) return
    setResettingQuotaId(subscriptionId)
    try {
      const res = await resetSubscriptionQuota(subscriptionId)
      if (res.success) {
        await selfSubscriptionQuery.refetch()
        setResetTarget(null)
        return
      }
      toast.error(res.message || t('Request failed'))
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setResettingQuotaId(null)
    }
  }

  const hasActive = activeSubscriptions.length > 0
  const hasAny = allSubscriptions.length > 0
  const isAvailable = loading || plans.length > 0 || hasAny

  useEffect(() => {
    onAvailabilityChange?.(isAvailable)
  }, [isAvailable, onAvailabilityChange])

  const planTitleMap = useMemo(() => {
    const map = new Map<number, string>()
    for (const p of plans) {
      if (p?.plan?.id) {
        map.set(p.plan.id, p.plan.title || '')
      }
    }
    return map
  }, [plans])

  const getRemainingDays = (sub: UserSubscriptionRecord) => {
    const endTime = getSubscriptionEndTime(sub?.subscription)
    if (!endTime) return 0
    const now = Date.now() / 1000
    return Math.max(0, Math.ceil((endTime - now) / 86400))
  }

  const getUsagePercent = (sub: UserSubscriptionRecord) => {
    const total = Number(sub?.subscription?.token_limit || 0)
    const used = Number(sub?.subscription?.token_used || 0)
    if (total <= 0) return 0
    return Math.round((used / total) * 100)
  }

  if (loading) {
    return (
      <Card className='gap-0 overflow-hidden py-0'>
        <CardHeader className='border-b p-3 !pb-3 sm:p-5 sm:!pb-5'>
          <Skeleton className='h-6 w-32' />
        </CardHeader>
        <CardContent className='space-y-4 p-3 sm:p-5'>
          <Skeleton className='h-20 w-full' />
          <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3'>
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className='h-48 w-full' />
            ))}
          </div>
        </CardContent>
      </Card>
    )
  }

  if (plans.length === 0 && !hasAny) {
    return null
  }

  return (
    <>
      <TitledCard
        title={t('Subscription Plans')}
        description={t('Subscribe to a plan for model access')}
        icon={<Crown className='h-4 w-4' />}
        contentClassName='space-y-4 sm:space-y-5'
      >
        <div className='rounded-xl border p-3 sm:p-4'>
          <div className='flex flex-wrap items-center justify-between gap-2.5 sm:gap-3'>
            <div className='flex min-w-0 flex-wrap items-center gap-2'>
              <span className='text-sm font-medium'>
                {t('My Subscriptions')}
              </span>
              <span className='flex items-center gap-1.5 text-xs font-medium'>
                <span
                  className={cn(
                    'size-1.5 shrink-0 rounded-full',
                    hasActive ? dotColorMap.success : dotColorMap.neutral
                  )}
                  aria-hidden='true'
                />
                {hasActive ? (
                  <span className={cn(textColorMap.success)}>
                    {activeSubscriptions.length} {t('active')}
                  </span>
                ) : (
                  <span className='text-muted-foreground'>
                    {t('No Active')}
                  </span>
                )}
                {allSubscriptions.length > activeSubscriptions.length && (
                  <>
                    <span className='text-muted-foreground/30'>·</span>
                    <span className='text-muted-foreground'>
                      {allSubscriptions.length - activeSubscriptions.length}{' '}
                      {t('expired')}
                    </span>
                  </>
                )}
              </span>
            </div>
            <div className='flex w-full items-center justify-between gap-2 sm:w-auto sm:justify-end'>
              <p className='text-muted-foreground text-xs sm:max-w-xs sm:text-right'>
                {t(
                  'API requests consume credits from your active subscription plan.'
                )}
              </p>
              <Button
                variant='ghost'
                size='icon'
                className='h-8 w-8 shrink-0'
                onClick={handleRefresh}
                disabled={refreshing}
              >
                <RefreshCw
                  className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`}
                />
              </Button>
            </div>
          </div>

          {selfSubscriptionData?.credit_balance && (
            <div className='mt-3 grid gap-2 rounded-md border p-3 text-xs sm:grid-cols-3'>
              <div>
                <div className='text-muted-foreground'>
                  {t('Available Credit balance')}
                </div>
                <div className='mt-1 font-medium'>
                  {formatFiniteCreditCount(
                    selfSubscriptionData.credit_balance.available_credit,
                    t
                  )}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>
                  {t('Settlement debt')}
                </div>
                <div className='mt-1 font-medium'>
                  {formatFiniteCreditCount(
                    selfSubscriptionData.credit_balance.settlement_debt,
                    t
                  )}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>
                  {t('Credit balance status')}
                </div>
                <div className='mt-1 font-medium'>
                  {t(selfSubscriptionData.credit_balance.status)}
                  {selfSubscriptionData.credit_balance.active
                    ? ` · ${t('Current active')}`
                    : ''}
                </div>
              </div>
              <div className='sm:col-span-3'>
                <CreditBalanceLedger
                  loadEntries={loadSelfCreditLedger}
                  initialEntries={
                    selfSubscriptionData.credit_balance_ledger || []
                  }
                />
              </div>
            </div>
          )}

          {hasAny && (
            <>
              <Separator className='my-3' />
              <div className='max-h-64 space-y-3 overflow-y-auto pr-1'>
                {allSubscriptions.map((sub) => {
                  const subscription = sub.subscription
                  const tokenLimit = Number(subscription?.token_limit || 0)
                  const tokenUsed = Number(subscription?.token_used || 0)
                  const remainTokens =
                    tokenLimit > 0 ? Math.max(0, tokenLimit - tokenUsed) : 0
                  const subscriptionLabel = getSubscriptionDisplayLabel(
                    sub,
                    planTitleMap,
                    t('Subscription')
                  )
                  const remainDays = getRemainingDays(sub)
                  const usagePercent = getUsagePercent(sub)
                  const now = Date.now() / 1000
                  const endTime = getSubscriptionEndTime(subscription)
                  const subscriptionId = subscription?.id || 0
                  const sourceLabel = getSubscriptionSourceLabel(sub, t)
                  const isCreditBalance =
                    subscription?.entitlement_type === 'credit_balance'
                  const isExpired = !isCreditBalance && endTime < now
                  const isCancelled = subscription?.status === 'cancelled'
                  const isActive =
                    subscription?.status === 'active' && !isExpired
                  const isSelected =
                    !!subscription?.is_active_selected ||
                    (!!activeSubscriptionId &&
                      subscriptionId === activeSubscriptionId)
                  const canResetQuota = canResetSubscriptionQuotaFromRecord(
                    sub,
                    now
                  )
                  const isSettingActive =
                    pendingActiveSubscriptionId === subscriptionId
                  const isResetting = resettingQuotaId === subscriptionId
                  const remainingEquivalentLabels =
                    renderSubscriptionChannelEquivalentLabels(
                      selfSubscriptionData,
                      isActive && isSelected,
                      t
                    )
                  const remainingEquivalentNotes =
                    renderSubscriptionChannelEquivalentNotes(
                      selfSubscriptionData,
                      isActive && isSelected,
                      t
                    )

                  return (
                    <div
                      key={subscription?.id}
                      className='bg-background rounded-md border p-3 text-xs'
                    >
                      <div className='flex flex-wrap items-start justify-between gap-2'>
                        <div className='flex min-w-0 flex-wrap items-center gap-2'>
                          <span className='font-medium'>
                            {subscriptionLabel}
                          </span>
                          {isActive ? (
                            <StatusBadge
                              label={t('Active')}
                              variant='success'
                              copyable={false}
                            />
                          ) : isCancelled ? (
                            <StatusBadge
                              label={t('Cancelled')}
                              variant='neutral'
                              copyable={false}
                            />
                          ) : (
                            <StatusBadge
                              label={t('Expired')}
                              variant='neutral'
                              copyable={false}
                            />
                          )}
                          {isActive && isSelected && (
                            <StatusBadge
                              label={t('Current active')}
                              variant='info'
                              copyable={false}
                            />
                          )}
                        </div>
                        {isActive && (
                          <div className='flex shrink-0 flex-wrap items-center justify-end gap-2'>
                            {!isCreditBalance && (
                              <span className='text-muted-foreground'>
                                {t('{{count}} days remaining', {
                                  count: remainDays,
                                })}
                              </span>
                            )}
                            {!isSelected && subscriptionId > 0 && (
                              <Button
                                variant='outline'
                                size='xs'
                                onClick={() =>
                                  handleSetActiveSubscription(subscriptionId)
                                }
                                disabled={isSettingActive}
                              >
                                {t('Set as active')}
                              </Button>
                            )}
                            {canResetQuota && (
                              <Button
                                variant='secondary'
                                size='xs'
                                onClick={() => setResetTarget(sub)}
                                disabled={isResetting}
                              >
                                {t('Reset credits')}
                              </Button>
                            )}
                          </div>
                        )}
                      </div>
                      <div className='text-muted-foreground mt-1.5'>
                        {t('Source')}: {sourceLabel}
                      </div>
                      {!isCreditBalance && (
                        <div className='text-muted-foreground mt-1.5'>
                          {isActive
                            ? t('Until')
                            : isCancelled
                              ? t('Cancelled at')
                              : t('Expired at')}{' '}
                          {new Date(endTime * 1000).toLocaleString()}
                        </div>
                      )}
                      {!isCreditBalance &&
                        isActive &&
                        (subscription?.next_reset_time ?? 0) > 0 && (
                          <div className='text-muted-foreground mt-1'>
                            {t('Next reset')}:{' '}
                            {new Date(
                              subscription!.next_reset_time! * 1000
                            ).toLocaleString()}
                          </div>
                        )}
                      <div className='text-muted-foreground mt-1'>
                        {isCreditBalance
                          ? t('Credit balance')
                          : t('Monthly Credits')}
                        :{' '}
                        {tokenLimit > 0 ? (
                          <Tooltip>
                            <TooltipTrigger
                              render={<span className='cursor-help' />}
                            >
                              {formatUsedCreditCount(tokenUsed, t)}/
                              {formatCreditLimit(tokenLimit, t)} ·{' '}
                              {t('Remaining')}{' '}
                              {formatFiniteCreditCount(remainTokens, t)}
                            </TooltipTrigger>
                            <TooltipContent>
                              {t('Raw Credits')}: {tokenUsed}/{tokenLimit} ·{' '}
                              {t('Remaining')} {remainTokens}
                            </TooltipContent>
                          </Tooltip>
                        ) : isCreditBalance ? (
                          formatFiniteCreditCount(0, t)
                        ) : (
                          formatCreditLimit(0, t)
                        )}
                        {tokenLimit > 0 && (
                          <span className='ml-2'>
                            {t('Used')} {usagePercent}%
                          </span>
                        )}
                      </div>
                      {remainingEquivalentLabels.length > 0 && (
                        <div className='text-muted-foreground mt-1 space-y-1 pl-3'>
                          <div>{t('Equivalent by channel')}:</div>
                          {remainingEquivalentLabels.map((label) => (
                            <div key={label}>{label}</div>
                          ))}
                          {remainingEquivalentNotes.map((note) => (
                            <div key={note}>{note}</div>
                          ))}
                        </div>
                      )}
                      <div className='text-muted-foreground mt-1'>
                        {t('Concurrency Limit')}:{' '}
                        {formatConcurrencyLimit(
                          subscription?.concurrency_limit,
                          t
                        )}
                      </div>
                      {tokenLimit > 0 && isActive && (
                        <Progress value={usagePercent} className='mt-2 h-1.5' />
                      )}
                    </div>
                  )
                })}
              </div>
            </>
          )}

          {selfSubscriptionData && (
            <SubscriptionBillingStrategyControl data={selfSubscriptionData} />
          )}

          {!hasAny && (
            <p className='text-muted-foreground mt-2 text-xs'>
              {t('Subscribe to a plan for model access')}
            </p>
          )}
        </div>

        {plans.length > 0 ? (
          <div className='grid grid-cols-1 gap-3 2xl:grid-cols-2 2xl:gap-4'>
            {plans.map((p, index) => {
              const plan = p?.plan
              if (!plan) return null
              const price = formatPlanPrice(plan.price_amount, plan.currency)
              const isPopular = index === 0 && plans.length > 1
              const planEquivalentLabels = renderPlanChannelEquivalentLabels(
                plan,
                t
              )
              const planEquivalentNotes = renderPlanChannelEquivalentNotes(
                plan,
                t
              )

              const benefits = [
                `${t('Validity Period')}: ${formatDuration(plan, t)}`,
                `${t('Monthly Credits')}: ${formatCreditLimit(
                  plan.monthly_token_limit,
                  t
                )}`,
                `${t('Concurrency Limit')}: ${formatConcurrencyLimit(
                  plan.concurrency_limit,
                  t
                )}`,
              ].filter(Boolean) as string[]

              return (
                <Card
                  key={plan.id}
                  className={cn(
                    'transition-shadow hover:shadow-md',
                    isPopular && 'border-primary/70 shadow-sm'
                  )}
                >
                  <CardContent className='flex h-full flex-col p-3.5 sm:p-4'>
                    <div className='mb-2 flex items-start justify-between gap-3'>
                      <div className='min-w-0'>
                        <h4 className='truncate font-semibold'>
                          {plan.title || t('Subscription Plans')}
                        </h4>
                        {plan.subtitle && (
                          <p className='text-muted-foreground truncate text-xs'>
                            {plan.subtitle}
                          </p>
                        )}
                      </div>
                      {isPopular && (
                        <StatusBadge
                          label={t('Popular')}
                          variant='info'
                          copyable={false}
                          className='shrink-0'
                        />
                      )}
                    </div>

                    <div className='py-2'>
                      <span className='text-primary text-2xl font-bold'>
                        {price}
                      </span>
                    </div>

                    <div className='flex-1 space-y-1.5 pb-3'>
                      {benefits.map((label) => (
                        <div
                          key={label}
                          className='text-muted-foreground flex items-center gap-2 text-xs'
                        >
                          <Check className='text-primary h-3 w-3 shrink-0' />
                          <span>{label}</span>
                        </div>
                      ))}
                      {planEquivalentLabels.length > 0 && (
                        <div className='text-muted-foreground space-y-1 pl-5 text-xs'>
                          <div>{t('Equivalent by channel')}:</div>
                          {planEquivalentLabels.map((label) => (
                            <div key={label}>{label}</div>
                          ))}
                          {planEquivalentNotes.map((note) => (
                            <div key={note}>{note}</div>
                          ))}
                        </div>
                      )}
                    </div>

                    <Separator className='mb-3' />

                    <Button
                      variant='outline'
                      className='w-full'
                      onClick={() => {
                        setSelectedPlan(p)
                        setPurchaseOpen(true)
                      }}
                    >
                      {t('Subscribe Now')}
                    </Button>
                  </CardContent>
                </Card>
              )
            })}
          </div>
        ) : (
          <p className='text-muted-foreground py-4 text-center text-sm'>
            {t('No plans available')}
          </p>
        )}
      </TitledCard>

      <SubscriptionPurchaseDialog
        open={purchaseOpen}
        onOpenChange={(open) => {
          setPurchaseOpen(open)
          if (!open) {
            void selfSubscriptionQuery.refetch()
          }
        }}
        plan={selectedPlan}
        enableStripe={enableStripe}
        enableCreem={enableCreem}
        enableOnlineTopUp={enableOnlineTopUp}
        enableKyrenSubscription={!!topupInfo?.enable_kyren_subscription}
        epayMethods={epayMethods}
        accountBalance={accountBalance}
        lastPurchaseMode={selfSubscriptionData?.last_subscription_purchase_mode}
        creditBalancePurchaseEnabled={
          selfSubscriptionData?.credit_balance_purchase_enabled
        }
        creditBalancePlan={selfSubscriptionData?.credit_balance_plan}
        onPurchaseSuccess={onPurchaseSuccess}
      />

      <ConfirmDialog
        open={!!resetTarget}
        onOpenChange={(open) => {
          if (!open) setResetTarget(null)
        }}
        title={t('Reset subscription credits')}
        desc={t(
          'Credit reset consumes one month from a paid plan and cannot be paid by invitation rewards.'
        )}
        confirmText={t('Reset credits')}
        isLoading={resettingQuotaId === resetTarget?.subscription?.id}
        handleConfirm={handleConfirmResetQuota}
      />
    </>
  )
}
