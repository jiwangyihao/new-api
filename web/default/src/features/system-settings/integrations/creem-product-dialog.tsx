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
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { CreemProduct } from '@/features/wallet/types'

function getCreemProductDialogSchema(t: TFunction) {
  return z.object({
    name: z.string().min(1, t('Product name is required')),
    productId: z.string().min(1, t('Product ID is required')),
    price: z.number().min(0.01, t('Price must be greater than 0')),
    balance_cny: z
      .string()
      .min(1, t('Amount is required'))
      .refine(
        (value) => accountBalanceCnyToCents(Number.parseFloat(value)) >= 1,
        { message: t('Amount must be at least 0.01 CNY') }
      ),
    currency: z.enum(['USD', 'EUR']),
  })
}

type CreemProductDialogFormValues = z.infer<ReturnType<typeof getCreemProductDialogSchema>>

// Re-export for backwards compatibility
export type CreemProductData = CreemProduct

export function creemProductToForm(
  product: Pick<CreemProductData, 'quota'>
): { balance_cny: string } {
  return {
    balance_cny: accountBalanceCentsToCnyAmount(product.quota).toFixed(2),
  }
}

export function creemProductFromForm(values: {
  balance_cny: string
}): Pick<CreemProductData, 'quota'> {
  return {
    quota: accountBalanceCnyToCents(Number.parseFloat(values.balance_cny)),
  }
}

type CreemProductDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: CreemProduct) => void
  editData?: CreemProduct | null
}

export function CreemProductDialog({
  open,
  onOpenChange,
  onSave,
  editData,
}: CreemProductDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!editData
  const formSchema = useMemo(() => getCreemProductDialogSchema(t), [t])


  const form = useForm<CreemProductDialogFormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      name: '',
      productId: '',
      price: 0,
      balance_cny: '0.00',
      currency: 'USD',
    },
  })

  useEffect(() => {
    if (editData) {
      form.reset({ ...editData, ...creemProductToForm(editData) })
    } else {
      form.reset({
        name: '',
        productId: '',
        price: 0,
        balance_cny: '0.00',
        currency: 'USD',
      })
    }
  }, [editData, form, open])

  const handleSubmit = (values: CreemProductDialogFormValues) => {
    const data: CreemProduct = {
      name: values.name,
      productId: values.productId,
      price: values.price,
      quota: creemProductFromForm(values).quota,
      currency: values.currency,
    }
    onSave(data)
    form.reset()
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-[500px]'>
        <DialogHeader>
          <DialogTitle>
            {isEditMode ? t('Edit product') : t('Add product')}
          </DialogTitle>
          <DialogDescription>
            {t('Configure a Creem product for credited account balance options.')}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleSubmit)}
            className='space-y-4'
          >
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Product Name')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('e.g., Basic Package')} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Display name shown to users.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='productId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Product ID')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g., prod_xxx')}
                      disabled={isEditMode}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Creem product ID from your Creem dashboard.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='currency'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Currency')}</FormLabel>
                    <Select
                      items={[
                        { value: 'USD', label: 'USD ($)' },
                        { value: 'EUR', label: 'EUR (€)' },
                      ]}
                      onValueChange={field.onChange}
                      value={field.value}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('Select currency')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='USD'>USD ($)</SelectItem>
                          <SelectItem value='EUR'>EUR (€)</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='price'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Price')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        min={0.01}
                        placeholder='10.00'
                        {...field}
                        onChange={(e) => field.onChange(e.target.valueAsNumber)}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

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

            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => onOpenChange(false)}
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
