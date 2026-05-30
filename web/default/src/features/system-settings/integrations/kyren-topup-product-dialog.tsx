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
import { useEffect } from 'react'
import * as z from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { type Resolver, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
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

const kyrenTopUpProductDialogSchema = z.object({
  id: z.string().min(1, 'Local top-up product ID is required'),
  name: z.string().min(1, 'Product name is required'),
  description: z.string(),
  product_id: z.string(),
  amount: z
    .string()
    .min(1, 'Amount is required')
    .refine((value) => normalizeKyrenAmountString(value) !== null, {
      message: 'Amount must be at least 0.01 CNY',
    }),
  quota: z.coerce.number().int().min(1, 'Quota must be at least 1'),
  enabled: z.boolean(),
})

type KyrenTopUpProductDialogFormValues = z.infer<
  typeof kyrenTopUpProductDialogSchema
>

export type KyrenTopUpProductData = KyrenTopUpProduct

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
  quota: 0,
  enabled: true,
}

export function KyrenTopUpProductDialog(props: KyrenTopUpProductDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!props.editData

  const form = useForm<KyrenTopUpProductDialogFormValues>({
    resolver: zodResolver(kyrenTopUpProductDialogSchema) as unknown as Resolver<KyrenTopUpProductDialogFormValues>,
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
        quota: props.editData.quota,
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
      quota: values.quota,
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
                name='quota'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Quota')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        placeholder={t('e.g., 500000')}
                        {...field}
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
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
