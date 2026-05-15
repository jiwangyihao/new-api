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
import { useEffect, useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import { DateTimePicker } from '@/components/datetime-picker'
import { createTrialCode, updateTrialCode } from '../api'
import {
  formValuesToTrialCodePayload,
  getTrialCodeFormSchema,
  TRIAL_CODE_FORM_DEFAULTS,
  trialCodeToFormValues,
  type TrialCodeFormValues,
} from '../lib'
import type { TrialCode } from '../types'
import { useTrialCodes } from './trial-codes-provider'

type TrialCodeMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: TrialCode
}

export function TrialCodeMutateDrawer(props: TrialCodeMutateDrawerProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useTrialCodes()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const isUpdate = Boolean(props.currentRow?.id)

  const form = useForm<TrialCodeFormValues>({
    resolver: zodResolver(getTrialCodeFormSchema(t)) as Resolver<TrialCodeFormValues>,
    defaultValues: TRIAL_CODE_FORM_DEFAULTS,
  })

  useEffect(() => {
    if (!props.open) return
    if (props.currentRow) {
      form.reset(trialCodeToFormValues(props.currentRow))
      return
    }
    form.reset(TRIAL_CODE_FORM_DEFAULTS)
  }, [form, props.currentRow, props.open])

  const onSubmit = async (values: TrialCodeFormValues) => {
    setIsSubmitting(true)
    try {
      const payload = formValuesToTrialCodePayload(values)
      const res = isUpdate
        ? await updateTrialCode(props.currentRow!.id, payload)
        : await createTrialCode(payload)
      if (res.success) {
        toast.success(
          isUpdate
            ? t('Trial code updated successfully')
            : t('Trial code created successfully')
        )
        props.onOpenChange(false)
        triggerRefresh()
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Sheet
      open={props.open}
      onOpenChange={(open) => {
        props.onOpenChange(open)
        if (!open) form.reset()
      }}
    >
      <SheetContent className='flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-[520px]'>
        <SheetHeader className='border-b px-4 py-3 text-start sm:px-6 sm:py-4'>
          <SheetTitle>
            {isUpdate ? t('Update trial code') : t('Create trial code')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Modify an existing trial code and its redemption rules.')
              : t('Create a manual trial code bound to a trial subscription plan.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='trial-code-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className='flex-1 space-y-4 overflow-y-auto px-3 py-3 pb-4 sm:space-y-6 sm:px-4'
          >
            <FormField
              control={form.control}
              name='code'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Trial code')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t('Enter trial code')} />
                  </FormControl>
                  <FormDescription>
                    {t('Users must manually enter this code during registration or OAuth account setup.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='plan_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Trial plan ID')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min={1}
                      onChange={(event) =>
                        field.onChange(parseInt(event.target.value, 10) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('The plan must be marked as a trial plan in subscription management.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='max_redemptions'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Max Redemptions')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={0}
                        onChange={(event) =>
                          field.onChange(parseInt(event.target.value, 10) || 0)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('0 means unlimited redemptions')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                    <div className='space-y-0.5'>
                      <FormLabel>{t('Enabled')}</FormLabel>
                      <FormDescription>
                        {t('Disabled trial codes cannot be redeemed.')}
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

            <FormField
              control={form.control}
              name='expires_at'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Expiration Time')}</FormLabel>
                  <FormControl>
                    <DateTimePicker
                      value={field.value}
                      onChange={field.onChange}
                      placeholder={t('Never expires')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Leave empty if this trial code should never expire.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>
        <SheetFooter className='gap-2 border-t px-4 py-3 sm:px-6 sm:py-4'>
          <SheetClose render={<Button variant='outline' type='button' />}>
            {t('Cancel')}
          </SheetClose>
          <Button type='submit' form='trial-code-form' disabled={isSubmitting}>
            {isSubmitting ? t('Saving...') : t('Save')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
