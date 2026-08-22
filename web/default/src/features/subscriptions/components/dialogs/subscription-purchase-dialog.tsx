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
import { useState, useEffect, useCallback } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Crown, CalendarClock, Package } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import type { TopupInfo } from '@/features/wallet/types'
import {
  paySubscriptionStripe,
  paySubscriptionCreem,
  paySubscriptionKyren,
  paySubscriptionEpay,
  getSubscriptionOrderStatus,
} from '../../api'
import {
  formatConcurrencyLimit,
  formatPlanPrice,
  formatDuration,
  formatCreditLimit,
  formatCompactCredit,
} from '../../lib'
import {
  creditPurchaseSuccessMessage,
  initialSubscriptionPurchaseMode,
  isCreditBalancePurchaseAvailable,
  purchaseModeSchema,
  type PurchaseModeFormValues,
} from '../../lib/subscription-purchase'
import { subscriptionQueryKeys } from '../../query-keys'
import type {
  PlanRecord,
  SubscriptionPayResponse,
  SubscriptionOrderStatus,
  SubscriptionPurchaseMode,
} from '../../types'

interface PaymentMethod {
  type: string
  name?: string
}

type KyrenReasonKey =
  | 'Kyren payment is unavailable'
  | 'Kyren product is not bound'
  | 'Kyren supports CNY subscription plans only'
  | 'Kyren does not support free subscription plans'
  | 'Kyren does not support trial subscription plans'
  | 'Kyren requires enabled and visible subscription plans'

interface KyrenAvailability {
  available: boolean
  reasonKey?: KyrenReasonKey
}

interface KyrenPaymentDependencies {
  planId: number
  purchaseMode: SubscriptionPurchaseMode
  paySubscriptionKyren: (data: {
    plan_id: number
    purchase_mode: SubscriptionPurchaseMode
  }) => Promise<SubscriptionPayResponse>
  openCheckout: (url: string) => void
  onOrderCreated?: (orderId: string) => void
}

interface CreditBalancePlanDisplay {
  concurrency_limit?: number
  queue_capacity?: number
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: PlanRecord | null
  enableStripe?: boolean
  enableCreem?: boolean
  enableOnlineTopUp?: boolean
  epayMethods?: PaymentMethod[]
  enableKyrenSubscription?: boolean
  lastPurchaseMode?: SubscriptionPurchaseMode
  creditBalancePurchaseEnabled?: boolean
  creditBalancePlan?: CreditBalancePlanDisplay | null
  onPurchaseSuccess?: () => Promise<void> | void
}

type ExternalPaymentProvider = 'stripe' | 'creem' | 'kyren' | 'epay'

interface ExternalCheckoutVariables {
  provider: ExternalPaymentProvider
  purchaseMode: SubscriptionPurchaseMode
  paymentMethod?: string
}

interface PendingExternalOrder {
  ownerUserId: number
  tradeNo: string
  provider: ExternalPaymentProvider
  purchaseMode: SubscriptionPurchaseMode
}

const pendingExternalOrderStoragePrefix =
  'new-api:subscription:pending-external-order:'

export function pendingExternalOrderStorageKey(
  userId: number,
  planId: number
): string {
  return `${pendingExternalOrderStoragePrefix}${userId}:${planId}`
}

function readPendingExternalOrder(
  userId: number,
  planId: number
): PendingExternalOrder | null {
  if (typeof window === 'undefined') return null
  const key = pendingExternalOrderStorageKey(userId, planId)
  try {
    const raw = window.sessionStorage.getItem(key)
    if (!raw) return null
    const candidate = JSON.parse(raw) as Record<string, unknown>
    const ownerUserId = Number(candidate.ownerUserId)
    const tradeNo =
      typeof candidate.tradeNo === 'string' ? candidate.tradeNo.trim() : ''
    const provider = candidate.provider
    const purchaseMode = candidate.purchaseMode
    if (
      ownerUserId !== userId ||
      !tradeNo ||
      !['stripe', 'creem', 'kyren', 'epay'].includes(String(provider)) ||
      !['timed', 'credit_balance'].includes(String(purchaseMode))
    ) {
      removePendingExternalOrder(userId, planId)
      return null
    }
    return {
      ownerUserId,
      tradeNo,
      provider: provider as ExternalPaymentProvider,
      purchaseMode: purchaseMode as SubscriptionPurchaseMode,
    }
  } catch {
    removePendingExternalOrder(userId, planId)
    return null
  }
}

function writePendingExternalOrder(
  userId: number,
  planId: number,
  order: PendingExternalOrder
): void {
  if (typeof window === 'undefined') return
  try {
    window.sessionStorage.setItem(
      pendingExternalOrderStorageKey(userId, planId),
      JSON.stringify(order)
    )
  } catch {
    // Polling still works for the current render when storage is unavailable.
  }
}

function removePendingExternalOrder(userId: number, planId: number): void {
  if (typeof window === 'undefined') return
  try {
    window.sessionStorage.removeItem(
      pendingExternalOrderStorageKey(userId, planId)
    )
  } catch {
    // Nothing else is required when storage is unavailable.
  }
}

function isKyrenSuccessMessage(message: string | undefined): boolean {
  return !message || message === 'success'
}

function getKyrenCheckoutUrl(res: SubscriptionPayResponse): string {
  return res.data?.checkout_url || res.data?.pay_link || res.url || ''
}

function isSafeHttpCheckoutUrl(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed) {
    return false
  }
  try {
    const url = new URL(trimmed)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export async function processKyrenSubscriptionPayment(
  deps: KyrenPaymentDependencies
): Promise<string> {
  const res = await deps.paySubscriptionKyren({
    plan_id: deps.planId,
    purchase_mode: deps.purchaseMode,
  })
  const checkoutUrl = getKyrenCheckoutUrl(res)
  const orderId = res.data?.order_id?.trim() || ''
  if (
    (res.success || isKyrenSuccessMessage(res.message)) &&
    checkoutUrl &&
    orderId &&
    isSafeHttpCheckoutUrl(checkoutUrl)
  ) {
    deps.onOrderCreated?.(orderId)
    deps.openCheckout(checkoutUrl)
    return orderId
  }
  throw new Error(
    res.message && !isKyrenSuccessMessage(res.message)
      ? res.message
      : 'Kyren checkout creation failed'
  )
}

export function getKyrenSubscriptionAvailability(
  plan: PlanRecord['plan'] | null | undefined,
  topupInfo: Pick<TopupInfo, 'enable_kyren_subscription'> | null | undefined
): KyrenAvailability {
  if (!topupInfo?.enable_kyren_subscription) {
    return { available: false, reasonKey: 'Kyren payment is unavailable' }
  }
  if (!plan?.kyren_product_id?.trim()) {
    return { available: false, reasonKey: 'Kyren product is not bound' }
  }
  if (String(plan.currency || '').toUpperCase() !== 'CNY') {
    return {
      available: false,
      reasonKey: 'Kyren supports CNY subscription plans only',
    }
  }
  if (Number(plan.price_amount || 0) < 0.01) {
    return {
      available: false,
      reasonKey: 'Kyren does not support free subscription plans',
    }
  }
  if (plan.is_trial === true) {
    return {
      available: false,
      reasonKey: 'Kyren does not support trial subscription plans',
    }
  }
  if (plan.enabled === false || plan.public_visible === false) {
    return {
      available: false,
      reasonKey: 'Kyren requires enabled and visible subscription plans',
    }
  }
  return { available: true }
}

function translateKyrenUnavailableReason(
  reasonKey: KyrenReasonKey | undefined,
  t: ReturnType<typeof useTranslation>['t']
): string {
  switch (reasonKey) {
    case 'Kyren product is not bound':
      return t('Kyren product is not bound')
    case 'Kyren supports CNY subscription plans only':
      return t('Kyren supports CNY subscription plans only')
    case 'Kyren does not support free subscription plans':
      return t('Kyren does not support free subscription plans')
    case 'Kyren does not support trial subscription plans':
      return t('Kyren does not support trial subscription plans')
    case 'Kyren requires enabled and visible subscription plans':
      return t('Kyren requires enabled and visible subscription plans')
    case 'Kyren payment is unavailable':
    default:
      return t('Kyren payment is unavailable')
  }
}

export function SubscriptionPurchaseDialog(props: Props) {
  const { t } = useTranslation()
  const { onOpenChange, onPurchaseSuccess } = props
  const [paying, setPaying] = useState(false)
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('')
  const plan = props.plan?.plan
  const creditAvailable = isCreditBalancePurchaseAvailable(
    plan,
    props.creditBalancePurchaseEnabled === true
  )
  const form = useForm<PurchaseModeFormValues>({
    resolver: zodResolver(purchaseModeSchema),
  })
  const purchaseMode = form.watch('purchase_mode')
  const queryClient = useQueryClient()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const [pendingExternalOrder, setPendingExternalOrder] =
    useState<PendingExternalOrder | null>(null)
  const planId = plan?.id
  const rememberPendingExternalOrder = useCallback(
    (order: Omit<PendingExternalOrder, 'ownerUserId'>) => {
      if (!userId) return
      const ownedOrder = { ...order, ownerUserId: userId }
      if (planId) writePendingExternalOrder(userId, planId, ownedOrder)
      setPendingExternalOrder(ownedOrder)
    },
    [planId, userId]
  )
  const clearPendingExternalOrder = useCallback(() => {
    if (userId && planId) removePendingExternalOrder(userId, planId)
    setPendingExternalOrder(null)
  }, [planId, userId])
  const externalCheckoutMutation = useMutation({
    mutationFn: async (variables: ExternalCheckoutVariables) => {
      if (!plan) {
        throw new Error('Payment request failed')
      }
      if (variables.provider === 'kyren') {
        const tradeNo = await processKyrenSubscriptionPayment({
          planId: plan.id,
          purchaseMode: variables.purchaseMode,
          paySubscriptionKyren,
          onOrderCreated: (orderId) =>
            rememberPendingExternalOrder({
              tradeNo: orderId,
              provider: variables.provider,
              purchaseMode: variables.purchaseMode,
            }),
          openCheckout: (url) => window.open(url, '_blank'),
        })
        return { tradeNo, ...variables }
      }

      const request = {
        plan_id: plan.id,
        purchase_mode: variables.purchaseMode,
      }
      let response: SubscriptionPayResponse
      let checkoutUrl: string
      if (variables.provider === 'stripe') {
        response = await paySubscriptionStripe(request)
        checkoutUrl = response.data?.pay_link || ''
      } else if (variables.provider === 'creem') {
        response = await paySubscriptionCreem(request)
        checkoutUrl = response.data?.checkout_url || ''
      } else {
        if (!variables.paymentMethod) {
          throw new Error('Please select a payment method')
        }
        response = await paySubscriptionEpay({
          ...request,
          payment_method: variables.paymentMethod,
        })
        checkoutUrl = response.url || ''
      }

      if (
        !(response.success === true || response.message === 'success') ||
        !isSafeHttpCheckoutUrl(checkoutUrl)
      ) {
        throw new Error(
          response.message && response.message !== 'success'
            ? response.message
            : 'Payment request failed'
        )
      }
      const tradeNo =
        response.data?.order_id?.trim() || response.order_id?.trim() || ''
      if (!tradeNo) {
        throw new Error('Payment request failed')
      }
      rememberPendingExternalOrder({
        tradeNo,
        provider: variables.provider,
        purchaseMode: variables.purchaseMode,
      })

      if (variables.provider === 'epay') {
        const paymentForm = document.createElement('form')
        paymentForm.action = checkoutUrl
        paymentForm.method = 'POST'
        const isSafari =
          typeof navigator !== 'undefined' &&
          /^((?!chrome|android).)*safari/i.test(navigator.userAgent)
        if (!isSafari) {
          paymentForm.target = '_blank'
        }
        Object.entries(
          (response.data || {}) as Record<string, unknown>
        ).forEach(([key, value]) => {
          if (key === 'order_id') return
          const input = document.createElement('input')
          input.type = 'hidden'
          input.name = key
          input.value = String(value)
          paymentForm.appendChild(input)
        })
        document.body.appendChild(paymentForm)
        paymentForm.submit()
        document.body.removeChild(paymentForm)
      } else {
        window.open(checkoutUrl, '_blank')
      }
      return { tradeNo, ...variables }
    },
    onMutate: () => setPaying(true),
    onSuccess: () => {
      toast.success(t('Payment page opened'))
    },
    onError: (error) => {
      const message = error instanceof Error ? error.message : ''
      toast.error(message ? t(message) : t('Payment request failed'))
    },
    onSettled: () => setPaying(false),
  })
  const activePendingExternalOrder =
    pendingExternalOrder?.ownerUserId === userId ? pendingExternalOrder : null
  const externalOrderQuery = useQuery({
    queryKey: [
      'subscriptions',
      userId || 'anonymous',
      'orders',
      activePendingExternalOrder?.tradeNo || 'idle',
    ],
    queryFn: () =>
      getSubscriptionOrderStatus(activePendingExternalOrder?.tradeNo || ''),
    enabled: props.open && activePendingExternalOrder !== null,
    refetchInterval: activePendingExternalOrder ? 1500 : false,
    retry: false,
  })
  const orderStatus: SubscriptionOrderStatus | undefined =
    externalOrderQuery.data?.data

  useEffect(() => {
    if (!activePendingExternalOrder || !orderStatus) return
    if (orderStatus.status === 'success') {
      clearPendingExternalOrder()
      const finishPurchase = async () => {
        if (
          orderStatus.purchase_mode === 'credit_balance' &&
          orderStatus.credit_balance
        ) {
          toast.success(
            creditPurchaseSuccessMessage(orderStatus.credit_balance, t)
          )
        } else {
          toast.success(t('Subscription purchased successfully'))
        }
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: subscriptionQueryKeys.selfSummary,
          }),
          queryClient.invalidateQueries({
            queryKey: subscriptionQueryKeys.dashboardSelfSubscriptions,
          }),
        ])
        await onPurchaseSuccess?.()
        onOpenChange(false)
      }
      void finishPurchase()
      return
    }
    if (orderStatus.status === 'failed' || orderStatus.status === 'expired') {
      clearPendingExternalOrder()
      toast.error(t('Payment failed or expired. You can try again.'))
    }
  }, [
    clearPendingExternalOrder,
    orderStatus,
    activePendingExternalOrder,
    onOpenChange,
    onPurchaseSuccess,
    queryClient,
    t,
  ])

  useEffect(() => {
    if (props.open && userId && planId) {
      setPendingExternalOrder(readPendingExternalOrder(userId, planId))
    } else {
      setPendingExternalOrder(null)
    }
  }, [planId, props.open, userId])

  useEffect(() => {
    if (props.open) {
      const initialMode = initialSubscriptionPurchaseMode(
        props.lastPurchaseMode,
        creditAvailable
      )
      form.reset(initialMode ? { purchase_mode: initialMode } : {})
    } else {
      form.reset({})
    }
  }, [creditAvailable, form, props.lastPurchaseMode, props.open])

  useEffect(() => {
    if (props.open && props.epayMethods && props.epayMethods.length > 0) {
      setSelectedEpayMethod(props.epayMethods[0].type)
    } else if (!props.open) {
      setSelectedEpayMethod('')
    }
  }, [props.open, props.epayMethods])

  if (!plan) return null

  const hasStripe = props.enableStripe && !!plan.stripe_price_id
  const hasCreem = props.enableCreem && !!plan.creem_product_id
  const hasKyren = !!props.enableKyrenSubscription
  const hasEpay =
    props.enableOnlineTopUp &&
    String(plan.currency || '').toUpperCase() === 'CNY' &&
    (props.epayMethods || []).length > 0
  const selectedEpayMethodLabel =
    (props.epayMethods || []).find((m) => m.type === selectedEpayMethod)
      ?.name ||
    selectedEpayMethod ||
    t('Select payment method')
  const price = formatPlanPrice(plan.price_amount, plan.currency)
  const kyrenAvailability = getKyrenSubscriptionAvailability(plan, {
    enable_kyren_subscription: hasKyren,
  })

  const submitExternalPayment = (
    provider: ExternalPaymentProvider,
    paymentMethod?: string
  ) => {
    if (!purchaseMode) {
      form.trigger('purchase_mode')
      return
    }
    if (purchaseMode === 'credit_balance' && !creditAvailable) {
      toast.error(t('Credit balance purchase is unavailable for this plan.'))
      return
    }
    if (provider === 'kyren' && !kyrenAvailability.available) {
      toast.error(
        translateKyrenUnavailableReason(kyrenAvailability.reasonKey, t)
      )
      return
    }
    if (provider === 'epay' && !paymentMethod) {
      toast.error(t('Please select a payment method'))
      return
    }
    externalCheckoutMutation.mutate({
      provider,
      purchaseMode,
      paymentMethod,
    })
  }


  return (
    <Dialog open={props.open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <Crown className='h-5 w-5' />
            {t('Purchase Subscription')}
          </DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <div className='space-y-3 sm:space-y-4'>
            <FormField
              control={form.control}
              name='purchase_mode'
              render={({ field }) => (
                <FormItem className='rounded-lg border p-3'>
                  <FormLabel>{t('Choose what this purchase adds')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Your saved choice is only a default. Confirm the mode for every purchase.'
                    )}
                  </FormDescription>
                  <FormControl>
                    <RadioGroup
                      value={field.value ?? ''}
                      onValueChange={(value) => field.onChange(value)}
                      className='gap-2'
                    >
                      <label className='hover:bg-muted/50 flex cursor-pointer items-start gap-3 rounded-md border p-3'>
                        <RadioGroupItem value='timed' />
                        <span className='grid gap-1'>
                          <span className='text-sm font-medium'>
                            {t('Timed subscription')}
                          </span>
                          <span className='text-muted-foreground text-xs'>
                            {t(
                              'Extends this plan and keeps its validity period, concurrency, and reset rules.'
                            )}
                          </span>
                        </span>
                      </label>
                      {creditAvailable && (
                        <label className='hover:bg-muted/50 flex cursor-pointer items-start gap-3 rounded-md border p-3'>
                          <RadioGroupItem value='credit_balance' />
                          <span className='grid gap-1'>
                            <span className='text-sm font-medium'>
                              {t('Credit balance')}
                            </span>
                            <span className='text-muted-foreground text-xs'>
                              {t(
                                'Adds {{credits}} permanent Credits using the global Credit balance service limits.',
                                {
                                  credits: formatCompactCredit(
                                    plan.monthly_token_limit || 0
                                  ),
                                }
                              )}
                            </span>
                          </span>
                        </label>
                      )}
                    </RadioGroup>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {purchaseMode === 'credit_balance' && props.creditBalancePlan && (
              <Alert>
                <AlertDescription>
                  {t('Credit balance service limits')}: {t('Concurrency Limit')}
                  : {props.creditBalancePlan.concurrency_limit || 0} ·{' '}
                  {t('Queue Capacity')}:{' '}
                  {props.creditBalancePlan.queue_capacity || 0}
                </AlertDescription>
              </Alert>
            )}

            <div className='bg-muted/50 space-y-2.5 rounded-lg border p-3 sm:space-y-3 sm:p-4'>
              <div className='flex justify-between'>
                <span className='text-muted-foreground text-sm'>
                  {t('Plan Name')}
                </span>
                <span className='max-w-[200px] truncate text-sm font-medium'>
                  {plan.title}
                </span>
              </div>
              {purchaseMode !== 'credit_balance' && (
                <div className='flex items-center justify-between'>
                  <span className='text-muted-foreground text-sm'>
                    {t('Validity Period')}
                  </span>
                  <span className='flex items-center gap-1 text-sm'>
                    <CalendarClock className='h-3.5 w-3.5' />
                    {formatDuration(plan, t)}
                  </span>
                </div>
              )}
              <div className='flex items-center justify-between'>
                <span className='text-muted-foreground text-sm'>
                  {purchaseMode === 'credit_balance'
                    ? t('Credits added')
                    : t('Monthly Credits')}
                </span>
                <span className='flex items-center gap-1 text-sm'>
                  <Package className='h-3.5 w-3.5' />
                  {formatCreditLimit(plan.monthly_token_limit, t)}
                </span>
              </div>
              {purchaseMode !== 'credit_balance' && (
                <div className='flex items-center justify-between'>
                  <span className='text-muted-foreground text-sm'>
                    {t('Concurrency Limit')}
                  </span>
                  <span className='text-sm'>
                    {formatConcurrencyLimit(plan.concurrency_limit, t)}
                  </span>
                </div>
              )}
              <Separator />
              <div className='flex items-center justify-between'>
                <span className='text-sm font-medium'>{t('Amount Due')}</span>
                <span className='text-primary text-lg font-bold'>{price}</span>
              </div>
            </div>

            {hasKyren && !kyrenAvailability.available && (
              <Alert>
                <AlertDescription>
                  {translateKyrenUnavailableReason(
                    kyrenAvailability.reasonKey,
                    t
                  )}
                </AlertDescription>
              </Alert>
            )}

            {activePendingExternalOrder && (
              <Alert aria-live='polite'>
                <AlertDescription className='flex flex-col gap-2'>
                  <span>
                    {externalOrderQuery.isError
                      ? t(
                          'Unable to check payment status. Retry status check or payment.'
                        )
                      : t(
                          'Waiting for payment confirmation. You can close this dialog and resume here later.'
                        )}
                  </span>
                  <span className='flex flex-wrap gap-2'>
                    {orderStatus?.checkout_url && (
                      <Button
                        type='button'
                        size='sm'
                        variant='outline'
                        onClick={() => window.open(orderStatus.checkout_url, '_blank')}
                      >
                        {t('Continue payment')}
                      </Button>
                    )}
                    {externalOrderQuery.isError && (
                      <>
                        <Button
                          type='button'
                          size='sm'
                          variant='outline'
                          onClick={() => void externalOrderQuery.refetch()}
                        >
                          {t('Retry status check')}
                        </Button>
                        <Button
                          type='button'
                          size='sm'
                          variant='outline'
                          onClick={clearPendingExternalOrder}
                        >
                          {t('Try payment again')}
                        </Button>
                      </>
                    )}
                  </span>
                </AlertDescription>
              </Alert>
            )}


            <div className='space-y-3'>
              <p className='text-muted-foreground text-xs'>
                {t('Select payment method')}
              </p>

              {(hasStripe || hasCreem || hasKyren) && (
                <div className='grid grid-cols-2 gap-2 sm:flex'>
                  {hasStripe && (
                    <Button
                      type='button'
                      variant='outline'
                      className='flex-1'
                      onClick={() => submitExternalPayment('stripe')}
                      disabled={paying || activePendingExternalOrder !== null}
                    >
                      Stripe
                    </Button>
                  )}
                  {hasCreem && (
                    <Button
                      type='button'
                      variant='outline'
                      className='flex-1'
                      onClick={() => submitExternalPayment('creem')}
                      disabled={paying || activePendingExternalOrder !== null}
                    >
                      Creem
                    </Button>
                  )}
                  {hasKyren && (
                    <Button
                      type='button'
                      variant='outline'
                      className='flex-1'
                      onClick={() => submitExternalPayment('kyren')}
                      disabled={
                        paying ||
                        activePendingExternalOrder !== null ||
                        !kyrenAvailability.available
                      }
                    >
                      {t('Pay with Kyren')}
                    </Button>
                  )}
                </div>
              )}

              {hasEpay && (
                <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-2'>
                  <Select
                    items={(props.epayMethods || []).map((method) => ({
                      value: method.type,
                      label: method.name || method.type,
                    }))}
                    value={selectedEpayMethod}
                    onValueChange={(value) =>
                      value !== null && setSelectedEpayMethod(value)
                    }
                    disabled={activePendingExternalOrder !== null}
                  >
                    <SelectTrigger className='flex-1'>
                      <SelectValue>{selectedEpayMethodLabel}</SelectValue>
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {(props.epayMethods || []).map((method) => (
                          <SelectItem key={method.type} value={method.type}>
                            {method.name || method.type}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <Button
                    type='button'
                    onClick={() =>
                      submitExternalPayment('epay', selectedEpayMethod)
                    }
                    disabled={
                      paying ||
                      !selectedEpayMethod ||
                      activePendingExternalOrder !== null
                    }
                  >
                    {t('Pay')}
                  </Button>
                </div>
              )}
            </div>
          </div>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
