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
import { useEffect, useMemo } from 'react'
import * as z from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { type Resolver, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  accountBalanceCentsToCnyAmount,
  accountBalanceCnyToCents,
} from '@/features/subscriptions/lib'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type { KyrenTopUpProduct } from '../types'
import { normalizeKyrenAmountString } from './kyren-topup-products-visual-editor'

function getKyrenTopUpProductDialogSchema(t: TFunction) {
  return z.object({
    id: z.string().min(1, t('Local top-up product ID is required')),
    name: z.string().min(1, t('Product name is required')),
    description: z.string(),
    product_id: z.string(),
    amount: z
      .string()
      .min(1, t('Amount is required'))
      .refine((value) => normalizeKyrenAmountString(value) !== null, {
        message: t('Amount must be at least 0.01 CNY'),
      }),
    balance_cny: z
      .string()
      .min(1, t('Amount is required'))
      .refine(
        (value) => accountBalanceCnyToCents(Number.parseFloat(value)) >= 1,
        { message: t('Amount must be at least 0.01 CNY') }
      ),
    enabled: z.boolean(),
  })
}

type KyrenTopUpProductDialogFormValues = z.infer<
  ReturnType<typeof getKyrenTopUpProductDialogSchema>
>

export type KyrenTopUpProductData = KyrenTopUpProduct

export function kyrenTopUpProductToForm(
  product: Pick<KyrenTopUpProduct, 'quota'>
): { balance_cny: string } {
  return {
    balance_cny: accountBalanceCentsToCnyAmount(product.quota).toFixed(2),
  }
}

export function kyrenTopUpProductFromForm(values: {
  balance_cny: string
}): Pick<KyrenTopUpProduct, 'quota'> {
  return {
    quota: accountBalanceCnyToCents(Number.parseFloat(values.balance_cny)),
  }
}

type KyrenTopUpProductDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: KyrenTopUpProduct) => void
  editData?: KyrenTopUpProduct | null
}

const emptyDefaults: KyrenTopUpProductDialogFormValues = {
  id: '',
  name: '',
  description: '',
  product_id: '',
  amount: '10.00',
  balance_cny: '0.00',
  enabled: true,
}

export function KyrenTopUpProductDialog(props: KyrenTopUpProductDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!props.editData

  const formSchema = useMemo(() => getKyrenTopUpProductDialogSchema(t), [t])

  const form = useForm<KyrenTopUpProductDialogFormValues>({
    resolver: zodResolver(formSchema) as unknown as Resolver<KyrenTopUpProductDialogFormValues>,
    defaultValues: emptyDefaults,
  })

  useEffect(() => {
    if (props.editData) {
      form.reset({
        id: props.editData.id,
        name: props.editData.name,
        description: props.editData.description ?? '',
        product_id: props.editData.product_id ?? '',
        amount: props.editData.amount,
        balance_cny: kyrenTopUpProductToForm(props.editData).balance_cny,
        enabled: props.editData.enabled,
      })
      return
    }
    form.reset(emptyDefaults)
  }, [props.editData, form, props.open])

  const handleSubmit = (values: KyrenTopUpProductDialogFormValues) => {
    props.onSave({
      id: values.id.trim(),
      name: values.name.trim(),
      description: values.description.trim(),
      product_id: values.product_id.trim(),
      amount: normalizeKyrenAmountString(values.amount) ?? values.amount.trim(),
      currency: 'CNY',
      quota: kyrenTopUpProductFromForm(values).quota,
      enabled: values.enabled,
    })
    form.reset(emptyDefaults)
    props.onOpenChange(false)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-[560px]'>
        <DialogHeader>
          <DialogTitle>
            {isEditMode
              ? t('Edit Kyren top-up product')
              : t('Add Kyren top-up product')}
          </DialogTitle>
          <DialogDescription>
            {t('Configure a fixed CNY wallet top-up product for Kyren Pay.')}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleSubmit)}
            className='space-y-4'
          >
            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Local product ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('e.g., topup_cny_10')}
                        disabled={isEditMode}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Stable local ID used by wallet checkout.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='product_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Kyren product ID')}</FormLabel>
                    <FormControl>
                      <Input placeholder={t('e.g., prod_xxx')} {...field} />
                    </FormControl>
                    <FormDescription>
                      {t('Leave blank and use Sync to create it in Kyren.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Product Name')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('e.g., CNY 10 Top-up')} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Display name shown to users and Kyren.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='description'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Description')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={3}
                      placeholder={t('Optional product description')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid gap-4 sm:grid-cols-3'>
              <FormItem>
                <FormLabel>{t('Currency')}</FormLabel>
                <FormControl>
                  <Input value='CNY' readOnly />
                </FormControl>
                <FormDescription>{t('Kyren top-up uses CNY.')}</FormDescription>
              </FormItem>

              <FormField
                control={form.control}
                name='amount'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Amount')}</FormLabel>
                    <FormControl>
                      <Input
                        inputMode='decimal'
                        placeholder='10.00'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='balance_cny'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Credited account balance (CNY)')}</FormLabel>
                    <FormControl>
                      <Input
                        inputMode='decimal'
                        placeholder={t('e.g., 39.90')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Saved to the server as CNY cents.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>{t('Enabled')}</FormLabel>
                    <FormDescription>
                      {t('Show this Kyren top-up product to users.')}
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

            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => props.onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit'>
                {isEditMode ? t('Update') : t('Add')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
