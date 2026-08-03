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
import { useEffect, useState, type ReactNode } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  AlertTriangle,
  CalendarClock,
  CreditCard,
  RefreshCw,
  Settings2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
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
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  createPlan,
  getSubscriptionKyrenProduct,
  syncSubscriptionKyrenProduct,
  updatePlan,
} from '../api'
import { getDurationUnitOptions, getResetPeriodOptions } from '../constants'
import {
  getPlanFormSchema,
  PLAN_FORM_DEFAULTS,
  planToFormValues,
  formValuesToPlanPayload,
  type PlanFormValues,
} from '../lib'
import type { PlanRecord, SubscriptionKyrenProductStatus } from '../types'
import { useSubscriptions } from './subscriptions-provider'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: PlanRecord
}

export function SubscriptionsMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: Props) {
  const { t } = useTranslation()
  const isEdit = !!currentRow?.plan?.id
  const { triggerRefresh } = useSubscriptions()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [kyrenStatus, setKyrenStatus] =
    useState<SubscriptionKyrenProductStatus | null>(null)
  const [isKyrenLoading, setIsKyrenLoading] = useState(false)
  const [isKyrenSyncing, setIsKyrenSyncing] = useState(false)
  const [riskConfirmed, setRiskConfirmed] = useState(false)
  const [riskReason, setRiskReason] = useState('')

  const schema = getPlanFormSchema(t)
  const form = useForm<PlanFormValues>({
    resolver: zodResolver(schema) as unknown as Resolver<PlanFormValues>,
    defaultValues: PLAN_FORM_DEFAULTS,
  })

  const loadKyrenStatus = async () => {
    if (!currentRow?.plan?.id) {
      setKyrenStatus(null)
      return
    }
    setIsKyrenLoading(true)
    try {
      const res = await getSubscriptionKyrenProduct(currentRow.plan.id)
      if (res.success) {
        setKyrenStatus(res.data || null)
        return
      }
      toast.error(res.message || t('Request failed'))
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setIsKyrenLoading(false)
    }
  }

  useEffect(() => {
    if (open) {
      if (currentRow?.plan) {
        form.reset(planToFormValues(currentRow.plan))
        void loadKyrenStatus()
      } else {
        form.reset(PLAN_FORM_DEFAULTS)
        setKyrenStatus(null)
      }
      setRiskConfirmed(false)
      setRiskReason('')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, currentRow, form])

  const durationUnit = form.watch('duration_unit')
  const resetPeriod = form.watch('quota_reset_period')
  const monthlyCredit = form.watch('monthly_token_limit')
  const existingTimedEntitlementCount = Number(
    currentRow?.existing_timed_entitlement_count || 0
  )
  const monthlyCreditChanged =
    isEdit &&
    Number(currentRow?.plan.monthly_token_limit || 0) !==
      Number(monthlyCredit || 0)
  const requiresRiskConfirmation =
    monthlyCreditChanged && existingTimedEntitlementCount > 0

  const onSubmit = async (values: PlanFormValues) => {
    setIsSubmitting(true)
    try {
      const payload = formValuesToPlanPayload(values)
      if (requiresRiskConfirmation) {
        if (!riskConfirmed || !riskReason.trim()) {
          toast.error(t('Confirm the renewal-merging risk and enter a reason.'))
          return
        }
        payload.risk_confirmed = true
        payload.risk_reason = riskReason.trim()
      }
      if (isEdit && currentRow?.plan?.id) {
        const res = await updatePlan(currentRow.plan.id, payload)
        if (res.success) {
          toast.success(t('Update succeeded'))
          onOpenChange(false)
          triggerRefresh()
        }
      } else {
        const res = await createPlan(payload)
        if (res.success) {
          toast.success(t('Create succeeded'))
          onOpenChange(false)
          triggerRefresh()
        }
      }
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleSyncKyrenProduct = async (
    mode: 'create_or_update' | 'create_new' | 'update_existing'
  ) => {
    if (!currentRow?.plan?.id) {
      return
    }
    setIsKyrenSyncing(true)
    try {
      const res = await syncSubscriptionKyrenProduct(currentRow.plan.id, mode)
      if (res.success) {
        toast.success(t('Kyren product synced'))
        form.setValue('kyren_product_id', res.data?.product_id || '', {
          shouldDirty: true,
        })
        await loadKyrenStatus()
        triggerRefresh()
        return
      }
      toast.error(res.message || t('Request failed'))
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setIsKyrenSyncing(false)
    }
  }
  const kyrenAlerts: ReactNode[] = []
  if (kyrenStatus?.missing || kyrenStatus?.bound === false) {
    kyrenAlerts.push(t('Kyren product is missing'))
  }
  if (kyrenStatus?.archived || kyrenStatus?.status === 'ARCHIVED') {
    kyrenAlerts.push(t('Kyren product is archived'))
  }
  if (kyrenStatus?.price_matches === false) {
    kyrenAlerts.push(t('Kyren product price mismatch'))
  }
  if (kyrenStatus?.currency_matches === false) {
    kyrenAlerts.push(t('Kyren product currency mismatch'))
  }

  const durationUnitOpts = getDurationUnitOptions(t)
  const resetPeriodOpts = getResetPeriodOptions(t)

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) {
          form.reset()
        }
      }}
    >
      <SheetContent className='flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-[600px]'>
        <SheetHeader className='border-b px-4 py-3 text-start sm:px-6 sm:py-4'>
          <SheetTitle>
            {isEdit ? t('Update plan info') : t('Create new subscription plan')}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t('Modify existing subscription plan configuration')
              : t(
                  'Fill in the following info to create a new subscription plan'
                )}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='subscription-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className='flex-1 space-y-4 overflow-y-auto px-3 py-3 pb-4 sm:space-y-6 sm:px-4'
          >
            {/* Basic Info */}
            <div className='space-y-4'>
              <h3 className='flex items-center gap-2 text-sm font-medium'>
                <Settings2 className='h-4 w-4' />
                {t('Basic Info')}
              </h3>

              <FormField
                control={form.control}
                name='title'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Plan Title')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t('e.g. Basic Plan')} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='subtitle'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Plan Subtitle')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('e.g. Suitable for light usage')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='price_amount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Actual Amount')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          onChange={(event) => {
                            field.onChange(event)
                            form.setValue('price_amount_changed', true)
                          }}
                          type='text'
                          inputMode='decimal'
                          placeholder='0.000000'
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='monthly_token_limit'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Monthly Credits')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={0}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t('0 means unlimited credits')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {requiresRiskConfirmation ? (
                  <Alert variant='destructive'>
                    <AlertTriangle />
                    <AlertTitle>{t('Monthly Credit change risk')}</AlertTitle>
                    <AlertDescription className='flex flex-col gap-3'>
                      <p>
                        {t(
                          'This plan has {{count}} active or conversion-grace timed entitlements. Renewal merging may make the converted Credit basis differ from each order refund basis.',
                          { count: existingTimedEntitlementCount }
                        )}
                      </p>
                      <label
                        className='flex items-start gap-2'
                        htmlFor='plan-credit-risk-confirmed'
                      >
                        <Checkbox
                          id='plan-credit-risk-confirmed'
                          checked={riskConfirmed}
                          onCheckedChange={(value) =>
                            setRiskConfirmed(value === true)
                          }
                          aria-labelledby='plan-credit-risk-confirmed-label'
                        />
                        <span id='plan-credit-risk-confirmed-label'>
                          {t('I accept the renewal-merging risk')}
                        </span>
                      </label>
                      <div className='flex flex-col gap-1'>
                        <label
                          htmlFor='plan-credit-risk-reason'
                          className='font-medium'
                        >
                          {t('Risk confirmation reason')}
                        </label>
                        <Textarea
                          id='plan-credit-risk-reason'
                          value={riskReason}
                          onChange={(event) =>
                            setRiskReason(event.target.value)
                          }
                          placeholder={t(
                            'Explain why this monthly Credit change is accepted'
                          )}
                          required
                          aria-required='true'
                        />
                      </div>
                    </AlertDescription>
                  </Alert>
                ) : null}
              </div>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='max_purchase_per_user'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Purchase Limit')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={0}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t('0 means unlimited')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='sort_order'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Sort Order')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 0)
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='enabled'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center gap-2 pt-8'>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormLabel className='!mt-0'>
                        {t('Enabled Status')}
                      </FormLabel>
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
                <FormField
                  control={form.control}
                  name='concurrency_limit'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Concurrency Limit')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={0}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t('0 means unlimited concurrency')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='queue_capacity'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Queue Capacity')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={0}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t('0 means use global queue capacity')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='gpt_abuse_warning_limit'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('GPT abuse warning limit')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={0}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          '0 means automatic: max(concurrency limit, system minimum)'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='business_code'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Business Code')}</FormLabel>
                      <FormControl>
                        <Input {...field} placeholder='basic_monthly' />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='is_trial'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center gap-2 rounded-md border p-3'>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormLabel className='!mt-0'>{t('Trial Plan')}</FormLabel>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='invite_trial'
                  render={({ field }) => (
                    <FormItem className='flex flex-col gap-2 rounded-md border p-3'>
                      <div className='flex flex-row items-center gap-2'>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                          />
                        </FormControl>
                        <FormLabel className='!mt-0'>
                          {t('Invite Trial Plan')}
                        </FormLabel>
                      </div>
                      <FormDescription>
                        {t(
                          'Use this trial plan as the default gift for invite code registrations.'
                        )}
                      </FormDescription>
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='unlimited_purchase_enabled'
                  render={({ field }) => (
                    <FormItem className='flex flex-col gap-2 rounded-md border p-3'>
                      <div className='flex flex-row items-center gap-2'>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                          />
                        </FormControl>
                        <FormLabel className='!mt-0'>
                          {t('Credit balance purchase eligible')}
                        </FormLabel>
                      </div>
                      <FormDescription>
                        {t(
                          'Allow this standard monthly timed plan to recharge the shared Credit balance.'
                        )}
                      </FormDescription>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='timed_conversion_enabled'
                  render={({ field }) => (
                    <FormItem className='flex flex-col gap-2 rounded-md border p-3'>
                      <div className='flex flex-row items-center gap-2'>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                          />
                        </FormControl>
                        <FormLabel className='!mt-0'>
                          {t('Timed plan conversion eligible')}
                        </FormLabel>
                      </div>
                      <FormDescription>
                        {t(
                          'Allow eligible timed entitlements from this plan to convert into Credit balance.'
                        )}
                      </FormDescription>
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='public_visible'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center gap-2 rounded-md border p-3'>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormLabel className='!mt-0'>
                        {t('Public Visible')}
                      </FormLabel>
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='trial_duration_hours'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Trial Duration Hours')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={0}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 0)
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='reward_eligible'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center gap-2 rounded-md border p-3'>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormLabel className='!mt-0'>
                      {t('Invitation Reward Eligible')}
                    </FormLabel>
                  </FormItem>
                )}
              />
            </div>

            {/* Duration Settings */}
            <div className='space-y-4'>
              <h3 className='flex items-center gap-2 text-sm font-medium'>
                <CalendarClock className='h-4 w-4' />
                {t('Duration Settings')}
              </h3>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='duration_unit'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Duration Unit')}</FormLabel>
                      <Select
                        items={[
                          ...durationUnitOpts.map((o) => ({
                            value: o.value,
                            label: o.label,
                          })),
                        ]}
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {durationUnitOpts.map((o) => (
                              <SelectItem key={o.value} value={o.value}>
                                {o.label}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {durationUnit === 'custom' ? (
                  <FormField
                    control={form.control}
                    name='custom_seconds'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Custom Seconds')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type='number'
                            min={1}
                            onChange={(e) =>
                              field.onChange(parseInt(e.target.value, 10) || 0)
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                ) : (
                  <FormField
                    control={form.control}
                    name='duration_value'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Duration Value')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type='number'
                            min={1}
                            onChange={(e) =>
                              field.onChange(parseInt(e.target.value, 10) || 0)
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}
              </div>
            </div>

            {/* Quota Reset */}
            <div className='space-y-4'>
              <h3 className='flex items-center gap-2 text-sm font-medium'>
                <RefreshCw className='h-4 w-4' />
                {t('Credit Reset')}
              </h3>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='quota_reset_period'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Reset Cycle')}</FormLabel>
                      <Select
                        items={[
                          ...resetPeriodOpts.map((o) => ({
                            value: o.value,
                            label: o.label,
                          })),
                        ]}
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {resetPeriodOpts.map((o) => (
                              <SelectItem key={o.value} value={o.value}>
                                {o.label}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='quota_reset_custom_seconds'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Custom Seconds')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={0}
                          disabled={resetPeriod !== 'custom'}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 0)
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Payment Config */}
            <div className='space-y-4'>
              <h3 className='flex items-center gap-2 text-sm font-medium'>
                <CreditCard className='h-4 w-4' />
                {t('Third-party Payment Config')}
              </h3>

              <FormField
                control={form.control}
                name='stripe_price_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Stripe Price ID</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder='price_...' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='creem_product_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Creem Product ID</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder='prod_...' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='kyren_product_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Kyren Product ID')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder='prod_...' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {isEdit ? (
                <div className='space-y-3 rounded-md border p-3'>
                  <div className='flex flex-wrap gap-2'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() => handleSyncKyrenProduct('create_new')}
                      disabled={isKyrenSyncing}
                    >
                      {t('Create Kyren product')}
                    </Button>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() => handleSyncKyrenProduct('create_or_update')}
                      disabled={isKyrenSyncing}
                    >
                      {t('Sync to Kyren')}
                    </Button>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={loadKyrenStatus}
                      disabled={isKyrenLoading}
                    >
                      {t('Refresh Kyren status')}
                    </Button>
                  </div>

                  {kyrenStatus ? (
                    <div className='bg-muted/40 grid grid-cols-2 gap-2 rounded-md p-3 text-xs'>
                      <span className='text-muted-foreground'>
                        {t('Product ID')}
                      </span>
                      <span>{kyrenStatus.product_id || '-'}</span>
                      <span className='text-muted-foreground'>
                        {t('Status')}
                      </span>
                      <span>{kyrenStatus.status || '-'}</span>
                      <span className='text-muted-foreground'>
                        {t('Price')}
                      </span>
                      <span>{kyrenStatus.price || '-'}</span>
                      <span className='text-muted-foreground'>
                        {t('Currency')}
                      </span>
                      <span>{kyrenStatus.currency || '-'}</span>
                    </div>
                  ) : (
                    <p className='text-muted-foreground text-xs'>
                      {t('No Kyren product status loaded')}
                    </p>
                  )}

                  {kyrenAlerts.map((message) => (
                    <Alert key={String(message)} variant='destructive'>
                      <AlertTriangle className='h-4 w-4' />
                      <AlertDescription>{message}</AlertDescription>
                    </Alert>
                  ))}
                </div>
              ) : (
                <Alert>
                  <AlertDescription>
                    {t('Save the plan before syncing Kyren product status.')}
                  </AlertDescription>
                </Alert>
              )}
            </div>
          </form>
        </Form>
        <SheetFooter className='grid grid-cols-2 gap-2 border-t px-4 py-3 sm:flex sm:px-6 sm:py-4'>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button
            form='subscription-form'
            type='submit'
            disabled={isSubmitting}
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
