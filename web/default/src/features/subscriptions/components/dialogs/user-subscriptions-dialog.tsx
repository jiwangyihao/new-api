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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import {
  getAdminPlans,
  getUserSubscriptions,
  createUserSubscription,
  invalidateUserSubscription,
  deleteUserSubscription,
} from '../../api'
import { formatPlanPrice, formatTimestamp } from '../../lib'
import type { PlanRecord, UserSubscriptionRecord } from '../../types'
import { AdminCreditBalancePanel } from '../admin-credit-balance-panel'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: { id: number; username?: string } | null
  onSuccess?: () => void
}

function SubscriptionStatusBadge(props: {
  sub: UserSubscriptionRecord['subscription']
  t: (key: string) => string
}) {
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now() / 1000
  const isExpired = (props.sub.end_time || 0) > 0 && props.sub.end_time < now
  const isActive = props.sub.status === 'active' && !isExpired
  if (props.sub.status === 'converted')
    return (
      <StatusBadge
        label={props.t('Converted')}
        variant='info'
        copyable={false}
      />
    )
  if (isActive)
    return (
      <StatusBadge
        label={props.t('Active')}
        variant='success'
        copyable={false}
      />
    )
  if (props.sub.status === 'cancelled')
    return (
      <StatusBadge
        label={props.t('Invalidated')}
        variant='neutral'
        copyable={false}
      />
    )
  return (
    <StatusBadge
      label={props.t('Expired')}
      variant='neutral'
      copyable={false}
    />
  )
}

function newTimedGrantIdempotencyKey(userId: number): string {
  const random =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `admin-timed-${userId}-${random}`
}

function isEligibleTimedGrantPlan(record: PlanRecord): boolean {
  const plan = record.plan
  const priceMicros = plan.price_amount_micros?.trim()
  if (
    !plan.enabled ||
    (plan.entitlement_type ?? 'timed') !== 'timed' ||
    plan.is_trial ||
    plan.invite_trial ||
    !priceMicros ||
    !/^\d+$/.test(priceMicros) ||
    !plan.currency?.trim()
  ) {
    return false
  }
  return BigInt(priceMicros) > 0n
}

export function UserSubscriptionsDialog(props: Props) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [plans, setPlans] = useState<PlanRecord[]>([])
  const [subs, setSubs] = useState<UserSubscriptionRecord[]>([])
  const [selectedPlanId, setSelectedPlanId] = useState<string>('')
  const [timedGrantReason, setTimedGrantReason] = useState('')
  const [timedGrantStatus, setTimedGrantStatus] = useState<{
    kind: 'idle' | 'failed' | 'succeeded'
    message: string
  }>({ kind: 'idle', message: '' })
  const timedGrantAttempt = useRef<{
    fingerprint: string
    idempotencyKey: string
  } | null>(null)
  const [confirmAction, setConfirmAction] = useState<{
    type: 'invalidate' | 'delete'
    subId: number
  } | null>(null)

  const planTitleMap = useMemo(() => {
    const map = new Map<number, string>()
    plans.forEach((p) => {
      if (p.plan.id) map.set(p.plan.id, p.plan.title || `#${p.plan.id}`)
    })
    return map
  }, [plans])

  const timedGrantPlans = useMemo(
    () => plans.filter(isEligibleTimedGrantPlan),
    [plans]
  )
  const selectedTimedGrantPlan = useMemo(
    () =>
      timedGrantPlans.find(
        (record) => String(record.plan.id) === selectedPlanId
      )?.plan ?? null,
    [selectedPlanId, timedGrantPlans]
  )

  const loadData = useCallback(async () => {
    if (!props.user?.id) return
    setLoading(true)
    try {
      const [plansRes, subsRes] = await Promise.all([
        getAdminPlans(),
        getUserSubscriptions(props.user.id),
      ])
      if (plansRes.success) setPlans(plansRes.data || [])
      if (subsRes.success) setSubs(subsRes.data || [])
    } catch {
      toast.error(t('Loading failed'))
    } finally {
      setLoading(false)
    }
  }, [props.user?.id, t])

  useEffect(() => {
    if (props.open && props.user?.id) {
      setSelectedPlanId('')
      setTimedGrantReason('')
      setTimedGrantStatus({ kind: 'idle', message: '' })
      timedGrantAttempt.current = null
      loadData()
    }
  }, [props.open, props.user?.id, loadData])

  const handleCreate = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const reason = timedGrantReason.trim()
    if (!props.user?.id || !selectedTimedGrantPlan || !reason) {
      toast.error(t('Select an eligible timed plan and enter a grant reason.'))
      return
    }
    const sourcePriceMicros = selectedTimedGrantPlan.price_amount_micros?.trim()
    const sourceCurrency = selectedTimedGrantPlan.currency.trim().toUpperCase()
    if (!sourcePriceMicros || !sourceCurrency) {
      toast.error(t('The selected plan has no precise valuation snapshot.'))
      return
    }
    const facts = {
      plan_id: selectedTimedGrantPlan.id,
      reason,
      source_price_micros: sourcePriceMicros,
      source_currency: sourceCurrency,
    }
    const fingerprint = JSON.stringify({ user_id: props.user.id, ...facts })
    if (timedGrantAttempt.current?.fingerprint !== fingerprint) {
      timedGrantAttempt.current = {
        fingerprint,
        idempotencyKey: newTimedGrantIdempotencyKey(props.user.id),
      }
    }
    setCreating(true)
    setTimedGrantStatus({ kind: 'idle', message: '' })
    try {
      const res = await createUserSubscription(props.user.id, {
        ...facts,
        idempotency_key: timedGrantAttempt.current.idempotencyKey,
      })
      if (res.success) {
        const message = res.data?.message || t('Timed grant succeeded')
        toast.success(message)
        setTimedGrantStatus({ kind: 'succeeded', message })
        setSelectedPlanId('')
        setTimedGrantReason('')
        timedGrantAttempt.current = null
        await loadData()
        props.onSuccess?.()
      } else {
        const message = res.message || t('Request failed')
        setTimedGrantStatus({ kind: 'failed', message })
        toast.error(message)
      }
    } catch {
      const message = t('Request failed')
      setTimedGrantStatus({ kind: 'failed', message })
      toast.error(message)
    } finally {
      setCreating(false)
    }
  }

  const handleConfirmAction = async () => {
    if (!confirmAction) return
    try {
      if (confirmAction.type === 'invalidate') {
        const res = await invalidateUserSubscription(confirmAction.subId)
        if (res.success) {
          toast.success(res.data?.message || t('Has been invalidated'))
          await loadData()
          props.onSuccess?.()
        }
      } else {
        const res = await deleteUserSubscription(confirmAction.subId)
        if (res.success) {
          toast.success(t('Deleted'))
          await loadData()
          props.onSuccess?.()
        }
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setConfirmAction(null)
    }
  }

  return (
    <>
      <Sheet open={props.open} onOpenChange={props.onOpenChange}>
        <SheetContent className='overflow-y-auto sm:max-w-2xl'>
          <SheetHeader>
            <SheetTitle>{t('User Subscription Management')}</SheetTitle>
            <SheetDescription>
              {props.user?.username || '-'} (ID: {props.user?.id || '-'})
            </SheetDescription>
          </SheetHeader>

          <Tabs defaultValue='subscriptions' className='mt-4'>
            <TabsList className='w-full'>
              <TabsTrigger value='subscriptions'>
                {t('Subscriptions')}
              </TabsTrigger>
              <TabsTrigger value='credit-finance'>
                {t('Credit finance')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='subscriptions' className='flex flex-col gap-4'>
              <form onSubmit={handleCreate}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor='timed-subscription-plan'>
                      {t('Timed subscription plan')}
                    </FieldLabel>
                    <Select
                      items={timedGrantPlans.map((record) => ({
                        value: String(record.plan.id),
                        label: `${record.plan.title} (${formatPlanPrice(
                          record.plan.price_amount,
                          record.plan.currency
                        )})`,
                      }))}
                      value={selectedPlanId}
                      onValueChange={(value) =>
                        value !== null && setSelectedPlanId(value)
                      }
                    >
                      <SelectTrigger
                        id='timed-subscription-plan'
                        aria-label={t('Timed subscription plan')}
                      >
                        <SelectValue placeholder={t('Select timed plan')} />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {timedGrantPlans.map((record) => (
                            <SelectItem
                              key={record.plan.id}
                              value={String(record.plan.id)}
                            >
                              {record.plan.title} (
                              {formatPlanPrice(
                                record.plan.price_amount,
                                record.plan.currency
                              )}
                              )
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FieldDescription>
                      {t(
                        'Only enabled paid timed plans with a precise price can be granted.'
                      )}
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='timed-grant-reason'>
                      {t('Grant reason')}
                    </FieldLabel>
                    <Textarea
                      id='timed-grant-reason'
                      value={timedGrantReason}
                      onChange={(event) =>
                        setTimedGrantReason(event.target.value)
                      }
                      placeholder={t('Describe the after-sales correction')}
                      disabled={creating}
                    />
                    <FieldDescription>
                      {t(
                        'Failed retries reuse the same idempotency key until grant details change.'
                      )}
                    </FieldDescription>
                  </Field>
                  <Field orientation='horizontal'>
                    <Button
                      type='submit'
                      disabled={
                        creating ||
                        !selectedTimedGrantPlan ||
                        !timedGrantReason.trim()
                      }
                    >
                      <Plus data-icon='inline-start' />
                      {t('Grant timed subscription')}
                    </Button>
                  </Field>
                  {timedGrantStatus.kind === 'failed' && (
                    <Alert variant='destructive'>
                      <AlertTitle>{t('Timed grant failed')}</AlertTitle>
                      <AlertDescription>
                        {timedGrantStatus.message}{' '}
                        {t(
                          'Retrying without changes reuses the same idempotency key.'
                        )}
                      </AlertDescription>
                    </Alert>
                  )}
                  {timedGrantStatus.kind === 'succeeded' && (
                    <Alert>
                      <AlertTitle>{t('Timed grant succeeded')}</AlertTitle>
                      <AlertDescription>
                        {timedGrantStatus.message}
                      </AlertDescription>
                    </Alert>
                  )}
                </FieldGroup>
              </form>

              <div className='rounded-md border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>ID</TableHead>
                      <TableHead>{t('Plan')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Validity')}</TableHead>
                      <TableHead>{t('Total Credits')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {loading ? (
                      <TableRow>
                        <TableCell colSpan={6} className='py-8 text-center'>
                          {t('Loading...')}
                        </TableCell>
                      </TableRow>
                    ) : subs.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={6}
                          className='text-muted-foreground py-8 text-center'
                        >
                          {t('No subscription records')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      subs.map((record) => {
                        const sub = record.subscription
                        const now = Date.now() / 1000
                        const isExpired =
                          (sub.end_time || 0) > 0 && sub.end_time < now
                        const isActive = sub.status === 'active' && !isExpired
                        const total = Number(sub.amount_total || 0)
                        const used = Number(sub.amount_used || 0)
                        const isConverted = sub.status === 'converted'

                        return (
                          <TableRow key={sub.id}>
                            <TableCell>#{sub.id}</TableCell>
                            <TableCell>
                              <div>
                                <div className='font-medium'>
                                  {planTitleMap.get(sub.plan_id) ||
                                    `#${sub.plan_id}`}
                                </div>
                                <div className='text-muted-foreground text-xs'>
                                  {t('Source')}: {sub.source || '-'}
                                </div>
                                {record.conversion_audit && (
                                  <div className='text-muted-foreground mt-1 flex flex-col gap-0.5 text-xs'>
                                    <div>
                                      {t('Conversion ID')}: #
                                      {record.conversion_audit.conversion_id}
                                    </div>
                                    <div>
                                      {t('Source subscription')}: #
                                      {
                                        record.conversion_audit
                                          .source_subscription_id
                                      }{' '}
                                      → {t('Target Credit balance')}: #
                                      {
                                        record.conversion_audit
                                          .target_subscription_id
                                      }
                                    </div>
                                    <div>
                                      {t('Status')}:{' '}
                                      {
                                        record.conversion_audit
                                          .source_status_before
                                      }{' '}
                                      →{' '}
                                      {
                                        record.conversion_audit
                                          .source_status_after
                                      }
                                    </div>
                                    <div>
                                      {t('Target Credit balance')}:{' '}
                                      {record.conversion_audit.target_status ||
                                        '-'}
                                    </div>
                                    <div>
                                      {t('Converted at')}:{' '}
                                      {formatTimestamp(
                                        Number(
                                          record.conversion_audit.converted_at
                                        )
                                      )}
                                    </div>
                                  </div>
                                )}
                              </div>
                            </TableCell>
                            <TableCell>
                              <SubscriptionStatusBadge sub={sub} t={t} />
                            </TableCell>
                            <TableCell>
                              <div className='text-xs'>
                                <div>
                                  {t('Start')}:{' '}
                                  {formatTimestamp(sub.start_time)}
                                </div>
                                <div>
                                  {t('End')}: {formatTimestamp(sub.end_time)}
                                </div>
                              </div>
                            </TableCell>
                            <TableCell>
                              {total > 0 ? `${used}/${total}` : t('Unlimited')}
                            </TableCell>
                            <TableCell className='text-right'>
                              <div className='flex justify-end gap-1'>
                                <Button
                                  size='sm'
                                  variant='outline'
                                  disabled={!isActive}
                                  onClick={() =>
                                    setConfirmAction({
                                      type: 'invalidate',
                                      subId: sub.id,
                                    })
                                  }
                                >
                                  {t('Invalidate')}
                                </Button>
                                <Button
                                  size='sm'
                                  variant='destructive'
                                  disabled={isConverted}
                                  onClick={() =>
                                    setConfirmAction({
                                      type: 'delete',
                                      subId: sub.id,
                                    })
                                  }
                                >
                                  {t('Delete')}
                                </Button>
                              </div>
                            </TableCell>
                          </TableRow>
                        )
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
            </TabsContent>
            <TabsContent value='credit-finance'>
              {props.user?.id ? (
                <AdminCreditBalancePanel
                  userId={props.user.id}
                  onSuccess={props.onSuccess}
                />
              ) : null}
            </TabsContent>
          </Tabs>
        </SheetContent>
      </Sheet>

      {confirmAction && (
        <ConfirmDialog
          open
          onOpenChange={(v) => !v && setConfirmAction(null)}
          title={
            confirmAction.type === 'invalidate'
              ? t('Confirm invalidate')
              : t('Confirm delete')
          }
          desc={
            confirmAction.type === 'invalidate'
              ? t(
                  'After invalidating, this subscription will be immediately deactivated. Historical records are not affected. Continue?'
                )
              : t(
                  'Deleting will permanently remove this subscription record (including benefit details). Continue?'
                )
          }
          handleConfirm={handleConfirmAction}
          destructive={confirmAction.type === 'delete'}
        />
      )}
    </>
  )
}
