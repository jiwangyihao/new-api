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
import * as React from 'react'
import * as z from 'zod'
import { type Resolver, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Code2, Eye } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
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
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  SettingsFormActionBar,
  SettingsFormSaveButton,
} from '../components/settings-form-actions'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { KyrenTopUpProduct } from '../types'
import { AmountDiscountVisualEditor } from './amount-discount-visual-editor'
import { AmountOptionsVisualEditor } from './amount-options-visual-editor'
import { CreemProductsVisualEditor } from './creem-products-visual-editor'
import {
  fetchKyrenTopUpProducts,
  KyrenTopUpProductsVisualEditor,
  parseKyrenTopUpProducts,
  saveKyrenTopUpProductsState,
  validateKyrenTopUpProducts,
  type KyrenTopUpProductStatus,
  type KyrenTopUpProductsListResponse,
} from './kyren-topup-products-visual-editor'
import { PaymentMethodsVisualEditor } from './payment-methods-visual-editor'
import {
  formatJsonForEditor,
  getJsonError,
  normalizeJsonForComparison,
  removeTrailingSlash,
} from './utils'
import {
  WaffoPancakeSettingsSection,
  type WaffoPancakeSettingsValues,
} from './waffo-pancake-settings-section'
import {
  WaffoSettingsSection,
  type WaffoSettingsValues,
} from './waffo-settings-section'

const paymentSchema = z.object({
  PayAddress: z.string().refine((value) => {
    const trimmed = value.trim()
    if (!trimmed) return true
    return /^https?:\/\//.test(trimmed)
  }, 'Provide a valid callback URL starting with http:// or https://'),
  EpayId: z.string(),
  EpayKey: z.string(),
  Price: z.coerce.number().min(0),
  MinTopUp: z.coerce.number().min(0),
  CustomCallbackAddress: z.string().refine((value) => {
    const trimmed = value.trim()
    if (!trimmed) return true
    return /^https?:\/\//.test(trimmed)
  }, 'Provide a valid URL starting with http:// or https://'),
  PayMethods: z.string().superRefine((value, ctx) => {
    const error = getJsonError(value)
    if (error) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: error })
    }
  }),
  AmountOptions: z.string().superRefine((value, ctx) => {
    const error = getJsonError(value, (parsed) => Array.isArray(parsed))
    if (error) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: error })
    }
  }),
  AmountDiscount: z.string().superRefine((value, ctx) => {
    const error = getJsonError(
      value,
      (parsed) =>
        !!parsed && typeof parsed === 'object' && !Array.isArray(parsed)
    )
    if (error) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: error })
    }
  }),
  StripeApiSecret: z.string(),
  StripeWebhookSecret: z.string(),
  StripePriceId: z.string(),
  StripeUnitPrice: z.coerce.number().min(0),
  StripeMinTopUp: z.coerce.number().min(0),
  StripePromotionCodesEnabled: z.boolean(),
  CreemApiKey: z.string(),
  CreemWebhookSecret: z.string(),
  CreemTestMode: z.boolean(),
  CreemProducts: z.string().superRefine((value, ctx) => {
    const error = getJsonError(value, (parsed) => Array.isArray(parsed))
    if (error) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: error })
    }
  }),
  KyrenApiKey: z.string(),
  KyrenWebhookSecret: z.string(),
  KyrenBaseURL: z.string().refine((value) => {
    const trimmed = value.trim()
    if (!trimmed) return true
    return /^https?:\/\//.test(trimmed)
  }, 'Provide a valid URL starting with http:// or https://'),
  KyrenTopUpProducts: z.string().superRefine((value, ctx) => {
    const error = getJsonError(value, (parsed) => Array.isArray(parsed))
    if (error) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: error })
      return
    }
    try {
      validateKyrenTopUpProducts(parseKyrenTopUpProducts(value))
    } catch (validationError) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message:
          validationError instanceof Error
            ? validationError.message
            : 'Invalid Kyren top-up products',
      })
    }
  }),
  ServerAddress: z.string(),
})

export type PaymentFormValues = z.infer<typeof paymentSchema>

type PaymentSettingsDefaultValues = Omit<
  PaymentFormValues,
  'KyrenTopUpProducts'
> & {
  KyrenTopUpProducts: KyrenTopUpProduct[]
}

type KyrenOptionValues = Pick<
  PaymentFormValues,
  'KyrenApiKey' | 'KyrenWebhookSecret' | 'KyrenBaseURL'
>
export type OptionUpdate = { key: string; value: string | number | boolean }

type PaymentOptionValues = Omit<PaymentFormValues, 'KyrenTopUpProducts'> & {
  KyrenTopUpProducts?: unknown
} & Partial<
    Pick<WaffoSettingsValues, 'WaffoMinTopUp' | 'WaffoUnitPrice'> &
      Pick<
        WaffoPancakeSettingsValues,
        'WaffoPancakeMinTopUp' | 'WaffoPancakeUnitPrice'
      >
  >

export function getKyrenWebhookUrl(serverAddress: string): string | null {
  const base = removeTrailingSlash(serverAddress)
  if (!base) return null
  return `${base}/api/kyren/webhook`
}

export function buildKyrenOptionUpdates(
  values: KyrenOptionValues,
  initial: KyrenOptionValues
): OptionUpdate[] {
  const sanitized = {
    KyrenApiKey: values.KyrenApiKey.trim(),
    KyrenWebhookSecret: values.KyrenWebhookSecret.trim(),
    KyrenBaseURL: removeTrailingSlash(values.KyrenBaseURL),
  }
  const initialValues = {
    KyrenApiKey: initial.KyrenApiKey.trim(),
    KyrenWebhookSecret: initial.KyrenWebhookSecret.trim(),
    KyrenBaseURL: removeTrailingSlash(initial.KyrenBaseURL),
  }
  const updates: OptionUpdate[] = []

  if (
    sanitized.KyrenApiKey &&
    sanitized.KyrenApiKey !== initialValues.KyrenApiKey
  ) {
    updates.push({ key: 'KyrenApiKey', value: sanitized.KyrenApiKey })
  }

  if (
    sanitized.KyrenWebhookSecret &&
    sanitized.KyrenWebhookSecret !== initialValues.KyrenWebhookSecret
  ) {
    updates.push({
      key: 'KyrenWebhookSecret',
      value: sanitized.KyrenWebhookSecret,
    })
  }

  if (sanitized.KyrenBaseURL !== initialValues.KyrenBaseURL) {
    updates.push({ key: 'KyrenBaseURL', value: sanitized.KyrenBaseURL })
  }

  return updates
}

export function buildPaymentOptionUpdates(
  values: PaymentOptionValues,
  initial: PaymentOptionValues
): OptionUpdate[] {
  const sanitized = {
    PayAddress: removeTrailingSlash(values.PayAddress),
    EpayId: values.EpayId.trim(),
    EpayKey: values.EpayKey.trim(),
    Price: values.Price,
    MinTopUp: values.MinTopUp,
    CustomCallbackAddress: removeTrailingSlash(values.CustomCallbackAddress),
    PayMethods: values.PayMethods.trim(),
    AmountOptions: values.AmountOptions.trim(),
    AmountDiscount: values.AmountDiscount.trim(),
    StripeApiSecret: values.StripeApiSecret.trim(),
    StripeWebhookSecret: values.StripeWebhookSecret.trim(),
    StripePriceId: values.StripePriceId.trim(),
    StripeUnitPrice: values.StripeUnitPrice,
    StripeMinTopUp: values.StripeMinTopUp,
    StripePromotionCodesEnabled: values.StripePromotionCodesEnabled,
    CreemApiKey: values.CreemApiKey.trim(),
    CreemWebhookSecret: values.CreemWebhookSecret.trim(),
    CreemTestMode: values.CreemTestMode,
    CreemProducts: values.CreemProducts.trim(),
  }

  const initialValues = {
    PayAddress: removeTrailingSlash(initial.PayAddress),
    EpayId: initial.EpayId.trim(),
    EpayKey: initial.EpayKey.trim(),
    Price: initial.Price,
    MinTopUp: initial.MinTopUp,
    CustomCallbackAddress: removeTrailingSlash(initial.CustomCallbackAddress),
    PayMethods: initial.PayMethods.trim(),
    AmountOptions: initial.AmountOptions.trim(),
    AmountDiscount: initial.AmountDiscount.trim(),
    StripeApiSecret: initial.StripeApiSecret.trim(),
    StripeWebhookSecret: initial.StripeWebhookSecret.trim(),
    StripePriceId: initial.StripePriceId.trim(),
    StripeUnitPrice: initial.StripeUnitPrice,
    StripeMinTopUp: initial.StripeMinTopUp,
    StripePromotionCodesEnabled: initial.StripePromotionCodesEnabled,
    CreemApiKey: initial.CreemApiKey.trim(),
    CreemWebhookSecret: initial.CreemWebhookSecret.trim(),
    CreemTestMode: initial.CreemTestMode,
    CreemProducts: initial.CreemProducts.trim(),
  }

  const updates: OptionUpdate[] = []

  if (sanitized.PayAddress !== initialValues.PayAddress) {
    updates.push({ key: 'PayAddress', value: sanitized.PayAddress })
  }

  if (sanitized.EpayId !== initialValues.EpayId) {
    updates.push({ key: 'EpayId', value: sanitized.EpayId })
  }

  if (sanitized.EpayKey && sanitized.EpayKey !== initialValues.EpayKey) {
    updates.push({ key: 'EpayKey', value: sanitized.EpayKey })
  }

  if (sanitized.Price !== initialValues.Price) {
    updates.push({ key: 'Price', value: sanitized.Price })
  }

  if (sanitized.MinTopUp !== initialValues.MinTopUp) {
    updates.push({ key: 'MinTopUp', value: sanitized.MinTopUp })
  }

  if (sanitized.CustomCallbackAddress !== initialValues.CustomCallbackAddress) {
    updates.push({
      key: 'CustomCallbackAddress',
      value: sanitized.CustomCallbackAddress,
    })
  }

  if (
    normalizeJsonForComparison(sanitized.PayMethods) !==
    normalizeJsonForComparison(initialValues.PayMethods)
  ) {
    updates.push({ key: 'PayMethods', value: sanitized.PayMethods })
  }

  if (
    normalizeJsonForComparison(sanitized.AmountOptions) !==
    normalizeJsonForComparison(initialValues.AmountOptions)
  ) {
    updates.push({
      key: 'payment_setting.amount_options',
      value: sanitized.AmountOptions,
    })
  }

  if (
    normalizeJsonForComparison(sanitized.AmountDiscount) !==
    normalizeJsonForComparison(initialValues.AmountDiscount)
  ) {
    updates.push({
      key: 'payment_setting.amount_discount',
      value: sanitized.AmountDiscount,
    })
  }

  if (
    sanitized.StripeApiSecret &&
    sanitized.StripeApiSecret !== initialValues.StripeApiSecret
  ) {
    updates.push({ key: 'StripeApiSecret', value: sanitized.StripeApiSecret })
  }

  if (
    sanitized.StripeWebhookSecret &&
    sanitized.StripeWebhookSecret !== initialValues.StripeWebhookSecret
  ) {
    updates.push({
      key: 'StripeWebhookSecret',
      value: sanitized.StripeWebhookSecret,
    })
  }

  if (sanitized.StripePriceId !== initialValues.StripePriceId) {
    updates.push({ key: 'StripePriceId', value: sanitized.StripePriceId })
  }

  if (sanitized.StripeUnitPrice !== initialValues.StripeUnitPrice) {
    updates.push({ key: 'StripeUnitPrice', value: sanitized.StripeUnitPrice })
  }

  if (sanitized.StripeMinTopUp !== initialValues.StripeMinTopUp) {
    updates.push({ key: 'StripeMinTopUp', value: sanitized.StripeMinTopUp })
  }

  if (
    sanitized.StripePromotionCodesEnabled !==
    initialValues.StripePromotionCodesEnabled
  ) {
    updates.push({
      key: 'StripePromotionCodesEnabled',
      value: sanitized.StripePromotionCodesEnabled,
    })
  }

  if (sanitized.CreemApiKey && sanitized.CreemApiKey !== initialValues.CreemApiKey) {
    updates.push({ key: 'CreemApiKey', value: sanitized.CreemApiKey })
  }

  if (
    sanitized.CreemWebhookSecret &&
    sanitized.CreemWebhookSecret !== initialValues.CreemWebhookSecret
  ) {
    updates.push({
      key: 'CreemWebhookSecret',
      value: sanitized.CreemWebhookSecret,
    })
  }

  if (sanitized.CreemTestMode !== initialValues.CreemTestMode) {
    updates.push({ key: 'CreemTestMode', value: sanitized.CreemTestMode })
  }

  if (
    normalizeJsonForComparison(sanitized.CreemProducts) !==
    normalizeJsonForComparison(initialValues.CreemProducts)
  ) {
    updates.push({ key: 'CreemProducts', value: sanitized.CreemProducts })
  }

  updates.push(...buildKyrenOptionUpdates(values, initial))

  if (
    values.WaffoMinTopUp !== undefined &&
    values.WaffoMinTopUp !== initial.WaffoMinTopUp
  ) {
    updates.push({ key: 'WaffoMinTopUp', value: values.WaffoMinTopUp })
  }

  if (
    values.WaffoUnitPrice !== undefined &&
    values.WaffoUnitPrice !== initial.WaffoUnitPrice
  ) {
    updates.push({ key: 'WaffoUnitPrice', value: values.WaffoUnitPrice })
  }

  if (
    values.WaffoPancakeMinTopUp !== undefined &&
    values.WaffoPancakeMinTopUp !== initial.WaffoPancakeMinTopUp
  ) {
    updates.push({
      key: 'WaffoPancakeMinTopUp',
      value: values.WaffoPancakeMinTopUp,
    })
  }

  if (
    values.WaffoPancakeUnitPrice !== undefined &&
    values.WaffoPancakeUnitPrice !== initial.WaffoPancakeUnitPrice
  ) {
    updates.push({
      key: 'WaffoPancakeUnitPrice',
      value: values.WaffoPancakeUnitPrice,
    })
  }

  return updates
}

function serializeKyrenTopUpProducts(
  products: KyrenTopUpProduct[] | string
): string {
  return JSON.stringify(
    validateKyrenTopUpProducts(parseKyrenProductsValue(products)),
    null,
    2
  )
}

function parseKyrenProductsValue(
  value: KyrenTopUpProduct[] | string
): KyrenTopUpProduct[] {
  if (typeof value === 'string') {
    return parseKyrenTopUpProducts(value)
  }
  return value
}

type PaymentSettingsSectionProps = {
  defaultValues: PaymentSettingsDefaultValues
  waffoDefaultValues: WaffoSettingsValues
  waffoPancakeDefaultValues: WaffoPancakeSettingsValues
}

export function PaymentSettingsSection({
  defaultValues,
  waffoDefaultValues,
  waffoPancakeDefaultValues,
}: PaymentSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const initialRef = React.useRef(defaultValues)
  const defaultsSignature = React.useMemo(
    () => JSON.stringify(defaultValues),
    [defaultValues]
  )

  const [payMethodsVisualMode, setPayMethodsVisualMode] = React.useState(true)
  const [amountOptionsVisualMode, setAmountOptionsVisualMode] =
    React.useState(true)
  const [amountDiscountVisualMode, setAmountDiscountVisualMode] =
    React.useState(true)
  const [creemProductsVisualMode, setCreemProductsVisualMode] =
    React.useState(true)
  const [kyrenStatuses, setKyrenStatuses] = React.useState<
    Record<string, KyrenTopUpProductStatus>
  >({})
  const [kyrenVersion, setKyrenVersion] = React.useState('')

  const form = useForm<PaymentFormValues>({
    resolver: zodResolver(paymentSchema) as unknown as Resolver<PaymentFormValues>,
    mode: 'onChange', // Enable real-time validation
    defaultValues: {
      ...defaultValues,
      PayMethods: formatJsonForEditor(defaultValues.PayMethods),
      AmountOptions: formatJsonForEditor(defaultValues.AmountOptions),
      AmountDiscount: formatJsonForEditor(defaultValues.AmountDiscount),
      CreemProducts: formatJsonForEditor(defaultValues.CreemProducts),
      KyrenTopUpProducts: serializeKyrenTopUpProducts(
        defaultValues.KyrenTopUpProducts
      ),
    },
  })
  const serverAddress = form.watch('ServerAddress')

  React.useEffect(() => {
    const parsedDefaults = JSON.parse(defaultsSignature) as PaymentFormValues
    const normalizedDefaults = {
      ...parsedDefaults,
      KyrenTopUpProducts: parseKyrenProductsValue(
        parsedDefaults.KyrenTopUpProducts
      ),
    }
    initialRef.current = normalizedDefaults
    form.reset({
      ...normalizedDefaults,
      PayMethods: formatJsonForEditor(normalizedDefaults.PayMethods),
      AmountOptions: formatJsonForEditor(normalizedDefaults.AmountOptions),
      AmountDiscount: formatJsonForEditor(normalizedDefaults.AmountDiscount),
      CreemProducts: formatJsonForEditor(normalizedDefaults.CreemProducts),
      KyrenTopUpProducts: serializeKyrenTopUpProducts(
        normalizedDefaults.KyrenTopUpProducts
      ),
    })
  }, [defaultsSignature, form])

  React.useEffect(() => {
    refetchKyrenTopUpProducts().catch(() => {
      toast.error(t('Failed to load Kyren top-up products'))
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const saveGeneralSettings = async () => {
    const updates = buildPaymentOptionUpdates(
      form.getValues(),
      initialRef.current
    ).filter(
      (update) =>
        update.key === 'Price' ||
        update.key === 'MinTopUp' ||
        update.key === 'PayMethods' ||
        update.key === 'payment_setting.amount_options' ||
        update.key === 'payment_setting.amount_discount'
    )

    if (updates.length === 0) {
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const saveEpaySettings = async () => {
    const updates = buildPaymentOptionUpdates(
      form.getValues(),
      initialRef.current
    ).filter(
      (update) =>
        update.key === 'PayAddress' ||
        update.key === 'EpayId' ||
        update.key === 'EpayKey' ||
        update.key === 'CustomCallbackAddress'
    )

    if (updates.length === 0) {
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const saveStripeSettings = async () => {
    const updates = buildPaymentOptionUpdates(
      form.getValues(),
      initialRef.current
    ).filter(
      (update) =>
        update.key === 'StripeApiSecret' ||
        update.key === 'StripeWebhookSecret' ||
        update.key === 'StripePriceId' ||
        update.key === 'StripeUnitPrice' ||
        update.key === 'StripeMinTopUp' ||
        update.key === 'StripePromotionCodesEnabled'
    )

    if (updates.length === 0) {
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const saveCreemSettings = async () => {
    const updates = buildPaymentOptionUpdates(
      form.getValues(),
      initialRef.current
    ).filter(
      (update) =>
        update.key === 'CreemApiKey' ||
        update.key === 'CreemWebhookSecret' ||
        update.key === 'CreemTestMode' ||
        update.key === 'CreemProducts'
    )

    if (updates.length === 0) {
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const refetchKyrenTopUpProducts =
    async (): Promise<KyrenTopUpProductsListResponse> => {
      const state = await fetchKyrenTopUpProducts()
      setKyrenVersion(state.version)
      form.setValue(
        'KyrenTopUpProducts',
        serializeKyrenTopUpProducts(state.products),
        {
          shouldDirty: false,
          shouldValidate: true,
        }
      )
      initialRef.current = {
        ...initialRef.current,
        KyrenTopUpProducts: state.products,
      }
      return state
    }

  const saveKyrenSettings = async () => {
    const values = form.getValues()
    const updates = buildKyrenOptionUpdates(values, initialRef.current)
    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
    if (updates.length > 0) {
      initialRef.current = {
        ...initialRef.current,
        KyrenApiKey: values.KyrenApiKey.trim() || initialRef.current.KyrenApiKey,
        KyrenWebhookSecret:
          values.KyrenWebhookSecret.trim() ||
          initialRef.current.KyrenWebhookSecret,
        KyrenBaseURL: removeTrailingSlash(values.KyrenBaseURL),
      }
    }

    const products = validateKyrenTopUpProducts(
      parseKyrenTopUpProducts(values.KyrenTopUpProducts)
    )
    const initialProducts = initialRef.current.KyrenTopUpProducts
    const topUpProductsChanged =
      JSON.stringify(products) !== JSON.stringify(initialProducts)
    if (!topUpProductsChanged) {
      return
    }

    const result = await saveKyrenTopUpProductsState({
      products,
      version: kyrenVersion,
      refetch: refetchKyrenTopUpProducts,
      notifyConflict: (message) => toast.error(t(message)),
    })
    setKyrenVersion(result.state.version)
    form.setValue(
      'KyrenTopUpProducts',
      serializeKyrenTopUpProducts(result.state.products),
      { shouldDirty: false, shouldValidate: true }
    )
    initialRef.current = {
      ...initialRef.current,
      KyrenTopUpProducts: result.state.products,
    }
  }

  const onSubmit = async (values: PaymentFormValues) => {
    const updates = buildPaymentOptionUpdates(values, initialRef.current)

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }

    const kyrenOptionsChanged = updates.some(
      (update) =>
        update.key === 'KyrenApiKey' ||
        update.key === 'KyrenWebhookSecret' ||
        update.key === 'KyrenBaseURL'
    )
    if (kyrenOptionsChanged) {
      initialRef.current = {
        ...initialRef.current,
        KyrenApiKey: values.KyrenApiKey.trim() || initialRef.current.KyrenApiKey,
        KyrenWebhookSecret:
          values.KyrenWebhookSecret.trim() ||
          initialRef.current.KyrenWebhookSecret,
        KyrenBaseURL: removeTrailingSlash(values.KyrenBaseURL),
      }
    }

    const products = validateKyrenTopUpProducts(
      parseKyrenTopUpProducts(values.KyrenTopUpProducts)
    )
    const initialProducts = initialRef.current.KyrenTopUpProducts
    if (JSON.stringify(products) === JSON.stringify(initialProducts)) {
      return
    }

    const result = await saveKyrenTopUpProductsState({
      products,
      version: kyrenVersion,
      refetch: refetchKyrenTopUpProducts,
      notifyConflict: (message) => toast.error(t(message)),
    })
    setKyrenVersion(result.state.version)
    form.setValue(
      'KyrenTopUpProducts',
      serializeKyrenTopUpProducts(result.state.products),
      { shouldDirty: false, shouldValidate: true }
    )
    initialRef.current = {
      ...initialRef.current,
      KyrenTopUpProducts: result.state.products,
    }
  }

  return (
    <SettingsSection
      title={t('Payment Gateway')}
      description={t(
        'Configure recharge pricing and payment gateway integrations'
      )}
    >
      {/* eslint-disable react-hooks/refs */}
      <Form {...form}>
        <SettingsFormActionBar>
          <SettingsFormSaveButton
            form='payment-settings-form'
            isSaving={updateOption.isPending}
            idleLabel={t('Save all settings')}
            savingLabel={t('Saving...')}
          />
        </SettingsFormActionBar>
        <form
          id='payment-settings-form'
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-8'
          data-no-autosubmit='true'
        >
          <div className='space-y-4'>
            <div>
              <h3 className='text-lg font-medium'>{t('General Settings')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Shared configuration for all payment gateways')}
              </p>
            </div>

            <div className='grid gap-6 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='Price'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Unit price for credited balance (CNY)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        min={0}
                        value={(field.value ?? 0) as number}
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Channel payment amount charged per CNY credited balance (Epay)'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='MinTopUp'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Minimum credited balance (CNY)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        min={0}
                        value={(field.value ?? 0) as number}
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Smallest credited account balance in CNY (Epay)')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='PayMethods'
              render={({ field }) => (
                <FormItem>
                  <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                    <FormLabel>{t('Payment methods')}</FormLabel>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() =>
                        setPayMethodsVisualMode(!payMethodsVisualMode)
                      }
                      className='w-full sm:w-auto'
                    >
                      {payMethodsVisualMode ? (
                        <>
                          <Code2 className='mr-2 h-3 w-3' />
                          {t('JSON Editor')}
                        </>
                      ) : (
                        <>
                          <Eye className='mr-2 h-3 w-3' />
                          {t('Visual Editor')}
                        </>
                      )}
                    </Button>
                  </div>
                  <FormControl>
                    {payMethodsVisualMode ? (
                      <PaymentMethodsVisualEditor
                        value={field.value}
                        onChange={field.onChange}
                      />
                    ) : (
                      <Textarea
                        rows={4}
                        placeholder={t(
                          '[{"name":"支付宝","type":"alipay","color":"#1677FF"}]'
                        )}
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    )}
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Configure available payment methods. Provide a JSON array.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid gap-6 md:grid-cols-2 md:items-start'>
              <FormField
                control={form.control}
                name='AmountOptions'
                render={({ field }) => (
                  <FormItem>
                    <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                      <FormLabel>{t('Account balance CNY options')}</FormLabel>
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={() =>
                          setAmountOptionsVisualMode(!amountOptionsVisualMode)
                        }
                        className='w-full sm:w-auto'
                      >
                        {amountOptionsVisualMode ? (
                          <>
                            <Code2 className='mr-2 h-3 w-3' />
                            {t('JSON Editor')}
                          </>
                        ) : (
                          <>
                            <Eye className='mr-2 h-3 w-3' />
                            {t('Visual Editor')}
                          </>
                        )}
                      </Button>
                    </div>
                    <FormControl>
                      {amountOptionsVisualMode ? (
                        <AmountOptionsVisualEditor
                          value={field.value}
                          onChange={field.onChange}
                        />
                      ) : (
                        <Textarea
                          rows={4}
                          placeholder='[10, 20, 50, 100]'
                          {...field}
                          onChange={(event) =>
                            field.onChange(event.target.value)
                          }
                        />
                      )}
                    </FormControl>
                    <FormDescription>
                      {t('Credited account balance CNY options (JSON array)')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='AmountDiscount'
                render={({ field }) => (
                  <FormItem>
                    <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                      <FormLabel>{t('Account balance CNY discount')}</FormLabel>
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={() =>
                          setAmountDiscountVisualMode(!amountDiscountVisualMode)
                        }
                        className='w-full sm:w-auto'
                      >
                        {amountDiscountVisualMode ? (
                          <>
                            <Code2 className='mr-2 h-3 w-3' />
                            {t('JSON Editor')}
                          </>
                        ) : (
                          <>
                            <Eye className='mr-2 h-3 w-3' />
                            {t('Visual Editor')}
                          </>
                        )}
                      </Button>
                    </div>
                    <FormControl>
                      {amountDiscountVisualMode ? (
                        <AmountDiscountVisualEditor
                          value={field.value}
                          onChange={field.onChange}
                        />
                      ) : (
                        <Textarea
                          rows={4}
                          placeholder='{"100":0.95,"200":0.9}'
                          {...field}
                          onChange={(event) =>
                            field.onChange(event.target.value)
                          }
                        />
                      )}
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Discount map by credited account balance CNY (JSON object)'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Button
              type='button'
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                saveGeneralSettings()
              }}
              disabled={updateOption.isPending}
            >
              {updateOption.isPending
                ? t('Saving...')
                : t('Save general settings')}
            </Button>
          </div>

          <Separator />

          <div className='space-y-4'>
            <div>
              <h3 className='text-lg font-medium'>{t('Epay Gateway')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Configuration for Epay payment integration')}
              </p>
            </div>

            <div className='grid gap-6 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='PayAddress'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Epay endpoint')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('https://pay.example.com')}
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Base address provided by your Epay service')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='CustomCallbackAddress'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Callback address')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('https://gateway.example.com')}
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Optional callback override. Leave blank to use server address'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className='grid gap-6 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='EpayId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Epay merchant ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='10001'
                        autoComplete='off'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='EpayKey'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Epay secret key')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Enter new key to update')}
                        autoComplete='new-password'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Leave blank unless rotating the secret')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Button
              type='button'
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                saveEpaySettings()
              }}
              disabled={updateOption.isPending}
            >
              {updateOption.isPending
                ? t('Saving...')
                : t('Save Epay settings')}
            </Button>
          </div>

          <Separator />

          <div className='space-y-4'>
            <div>
              <h3 className='text-lg font-medium'>{t('Stripe Gateway')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Configuration for Stripe payment integration')}
              </p>
            </div>

            <div className='rounded-md bg-blue-50 p-4 text-sm text-blue-900 dark:bg-blue-950 dark:text-blue-100'>
              <p className='mb-2 font-medium'>{t('Webhook Configuration:')}</p>
              <ul className='list-inside list-disc space-y-1'>
                <li>
                  {t('Webhook URL:')}{' '}
                  <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                    {'<ServerAddress>/api/stripe/webhook'}
                  </code>
                </li>
                <li>
                  {t('Required events:')}{' '}
                  <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                    {t('checkout.session.completed')}
                  </code>{' '}
                  {t('and')}{' '}
                  <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                    {t('checkout.session.expired')}
                  </code>
                </li>
                <li>
                  {t('Configure at:')}{' '}
                  <a
                    href='https://dashboard.stripe.com/developers'
                    target='_blank'
                    rel='noreferrer'
                    className='underline hover:no-underline'
                  >
                    {t('Stripe Dashboard')}
                  </a>
                </li>
              </ul>
            </div>

            <div className='grid gap-6 md:grid-cols-3'>
              <FormField
                control={form.control}
                name='StripeApiSecret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('API secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('sk_xxx or rk_xxx')}
                        autoComplete='new-password'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Stripe API key (leave blank unless updating)')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='StripeWebhookSecret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Webhook secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('whsec_xxx')}
                        autoComplete='new-password'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Webhook signing secret (leave blank unless updating)'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='StripePriceId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Price ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('price_xxx')}
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Stripe product price ID')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className='grid gap-6 md:grid-cols-3'>
              <FormField
                control={form.control}
                name='StripeUnitPrice'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Unit price for credited balance (CNY)')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        min={0}
                        value={(field.value ?? 0) as number}
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Channel payment amount charged per CNY credited balance')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='StripeMinTopUp'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Minimum credited balance (CNY)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        min={0}
                        value={(field.value ?? 0) as number}
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Minimum credited account balance in CNY')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='StripePromotionCodesEnabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-base'>
                        {t('Promotion codes')}
                      </FormLabel>
                      <FormDescription>
                        {t('Allow users to enter promo codes')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>

            <Button
              type='button'
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                saveStripeSettings()
              }}
              disabled={updateOption.isPending}
            >
              {updateOption.isPending
                ? t('Saving...')
                : t('Save Stripe settings')}
            </Button>
          </div>

          <Separator />

          <div className='space-y-4'>
            <div>
              <h3 className='text-lg font-medium'>{t('Creem Gateway')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Configuration for Creem payment integration')}
              </p>
            </div>

            <div className='rounded-md bg-blue-50 p-4 text-sm text-blue-900 dark:bg-blue-950 dark:text-blue-100'>
              <p className='mb-2 font-medium'>{t('Webhook Configuration:')}</p>
              <ul className='list-inside list-disc space-y-1'>
                <li>
                  {t('Webhook URL:')}{' '}
                  <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                    {'<ServerAddress>/api/creem/webhook'}
                  </code>
                </li>
                <li>{t('Configure in your Creem dashboard')}</li>
              </ul>
            </div>

            <div className='grid gap-6 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='CreemApiKey'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('API Key')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Enter Creem API key')}
                        autoComplete='new-password'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Creem API key (leave blank unless updating)')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='CreemWebhookSecret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Webhook Secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Enter webhook secret')}
                        autoComplete='new-password'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Webhook signing secret (leave blank unless updating)'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='CreemTestMode'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Test Mode')}
                    </FormLabel>
                    <FormDescription>
                      {t('Enable test mode for Creem payments')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='CreemProducts'
              render={({ field }) => (
                <FormItem>
                  <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                    <FormLabel>{t('Products')}</FormLabel>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() =>
                        setCreemProductsVisualMode(!creemProductsVisualMode)
                      }
                      className='w-full sm:w-auto'
                    >
                      {creemProductsVisualMode ? (
                        <>
                          <Code2 className='mr-2 h-3 w-3' />
                          {t('JSON Editor')}
                        </>
                      ) : (
                        <>
                          <Eye className='mr-2 h-3 w-3' />
                          {t('Visual Editor')}
                        </>
                      )}
                    </Button>
                  </div>
                  <FormControl>
                    {creemProductsVisualMode ? (
                      <CreemProductsVisualEditor
                        value={field.value}
                        onChange={field.onChange}
                      />
                    ) : (
                      <Textarea
                        rows={4}
                        placeholder='[{"name":"Basic","productId":"prod_xxx","price":10,"quota":3990,"currency":"USD"}]'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    )}
                  </FormControl>
                  <FormDescription>
                    {t('Configure Creem products. Provide a JSON array.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <Button
              type='button'
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                saveCreemSettings()
              }}
              disabled={updateOption.isPending}
            >
              {updateOption.isPending
                ? t('Saving...')
                : t('Save Creem settings')}
            </Button>
          </div>


          <Separator />

          <div className='space-y-4'>
            <div>
              <h3 className='text-lg font-medium'>{t('Kyren Gateway')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Configuration for Kyren Pay integration')}
              </p>
            </div>

            <div className='rounded-md bg-blue-50 p-4 text-sm text-blue-900 dark:bg-blue-950 dark:text-blue-100'>
              <p className='mb-2 font-medium'>{t('Webhook Configuration:')}</p>
              <ul className='list-inside list-disc space-y-1'>
                <li>
                  {t('Webhook URL:')}{' '}
                  <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                    {getKyrenWebhookUrl(serverAddress) ??
                      t('Server address is not configured')}
                  </code>
                </li>
                <li>
                  {t(
                    'Configure ServerAddress after deployment to expose the Kyren webhook URL.'
                  )}
                </li>
              </ul>
            </div>

            <div className='grid gap-6 md:grid-cols-3'>
              <FormField
                control={form.control}
                name='KyrenApiKey'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('API Key')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Enter Kyren API key')}
                        autoComplete='new-password'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Kyren API key (leave blank unless updating)')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='KyrenWebhookSecret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Webhook Secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Enter webhook secret')}
                        autoComplete='new-password'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Webhook signing secret (leave blank unless updating)'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='KyrenBaseURL'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Base URL')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='https://api.kyren.top'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Kyren API base URL. Trailing slashes are removed on save.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='KyrenTopUpProducts'
              render={({ field }) => (
                <FormItem>
                  <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                    <FormLabel>{t('Kyren top-up products')}</FormLabel>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={(event) => {
                        event.preventDefault()
                        event.stopPropagation()
                        saveKyrenSettings()
                      }}
                      disabled={updateOption.isPending}
                      className='w-full sm:w-auto'
                    >
                      {updateOption.isPending
                        ? t('Saving...')
                        : t('Save Kyren settings')}
                    </Button>
                  </div>
                  <FormControl>
                    <KyrenTopUpProductsVisualEditor
                      products={parseKyrenTopUpProducts(field.value)}
                      version={kyrenVersion}
                      statuses={kyrenStatuses}
                      onChange={(products) =>
                        field.onChange(serializeKyrenTopUpProducts(products))
                      }
                      onVersionChange={setKyrenVersion}
                      onStatusesChange={setKyrenStatuses}
                      onRefetch={refetchKyrenTopUpProducts}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Configure fixed CNY Kyren top-up products. They are saved through the dedicated Kyren API.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <Button type='submit' disabled={updateOption.isPending}>
            {updateOption.isPending ? t('Saving...') : t('Save all settings')}
          </Button>
        </form>
      </Form>

      <Separator />

      <WaffoSettingsSection defaultValues={waffoDefaultValues} />

      <Separator />

      <WaffoPancakeSettingsSection defaultValues={waffoPancakeDefaultValues} />
      {/* eslint-enable react-hooks/refs */}
    </SettingsSection>
  )
}
