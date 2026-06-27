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
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
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
import { MultiSelect } from '@/components/multi-select'
import {
  createChannelGroup,
  getChannelOptions,
  updateChannelGroup,
} from '../api'
import {
  CHANNEL_GROUP_FORM_DEFAULTS,
  channelGroupFormSchema,
  channelGroupToFormValues,
  formValuesToPayload,
  type ChannelGroup,
  type ChannelGroupFormValues,
} from '../types'

type ChannelGroupMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: ChannelGroup
}

export const CHANNEL_GROUPS_QUERY_KEY = ['channel-groups'] as const

export function ChannelGroupMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ChannelGroupMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const isDefault = currentRow?.is_default === true
  const queryClient = useQueryClient()
  const [isSubmitting, setIsSubmitting] = useState(false)

  const { data: channelOptions } = useQuery({
    queryKey: ['channel-options'],
    queryFn: getChannelOptions,
    staleTime: 5 * 60 * 1000,
  })

  const channels = channelOptions ?? []

  const form = useForm<ChannelGroupFormValues>({
    resolver: zodResolver(channelGroupFormSchema),
    defaultValues: CHANNEL_GROUP_FORM_DEFAULTS,
  })

  useEffect(() => {
    if (open && isUpdate && currentRow) {
      form.reset(channelGroupToFormValues(currentRow))
    } else if (open && !isUpdate) {
      form.reset(CHANNEL_GROUP_FORM_DEFAULTS)
    }
  }, [open, isUpdate, currentRow, form])

  const mutation = useMutation({
    mutationFn: async (values: ChannelGroupFormValues) => {
      const payload = formValuesToPayload(values, currentRow?.id)
      if (isUpdate && currentRow) {
        return updateChannelGroup({ ...payload, id: currentRow.id })
      }
      return createChannelGroup(payload)
    },
    onSuccess: (result) => {
      if (result.success) {
        toast.success(
          isUpdate
            ? t('Channel group updated successfully')
            : t('Channel group created successfully')
        )
        queryClient.invalidateQueries({ queryKey: CHANNEL_GROUPS_QUERY_KEY })
        onOpenChange(false)
      } else {
        toast.error(result.message || t('Operation failed'))
      }
    },
    onError: () => {
      toast.error(t('Operation failed'))
    },
    onSettled: () => setIsSubmitting(false),
  })

  const onSubmit = (values: ChannelGroupFormValues) => {
    setIsSubmitting(true)
    mutation.mutate(values)
  }

  const billingMode = form.watch('credit_billing_mode')

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) form.reset()
      }}
    >
      <SheetContent
        side='right'
        className='bg-background flex !h-dvh !w-screen max-w-none gap-0 overflow-hidden p-0 sm:!w-full sm:!max-w-[560px]'
      >
        <SheetHeader className='bg-background border-b px-4 py-3 text-start sm:px-5 sm:py-4'>
          <SheetTitle className='text-base sm:text-lg'>
            {isUpdate ? t('Update Channel Group') : t('Create Channel Group')}
          </SheetTitle>
          <SheetDescription className='pr-6 text-xs sm:text-sm'>
            {t(
              'Bundle channels into a group. Users only see groups, not upstream channels.'
            )}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='channel-group-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className='min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4'
          >
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Name')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      readOnly={isDefault}
                      placeholder={t('Enter a name')}
                    />
                  </FormControl>
                  {isDefault && (
                    <FormDescription>
                      {t('The default group name cannot be changed.')}
                    </FormDescription>
                  )}
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
                      {...field}
                      className='min-h-16 resize-none'
                      placeholder={t('Optional description shown to users')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='credit_billing_mode'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Billing Mode')}</FormLabel>
                  <FormControl>
                    <NativeSelect
                      className='w-full'
                      value={field.value}
                      onChange={(e) => field.onChange(e.target.value)}
                    >
                      <NativeSelectOption value=''>
                        {t('Inherit from channel')}
                      </NativeSelectOption>
                      <NativeSelectOption value='usage_tokens'>
                        {t('Usage tokens')}
                      </NativeSelectOption>
                      <NativeSelectOption value='fixed_request'>
                        {t('Fixed per request')}
                      </NativeSelectOption>
                    </NativeSelect>
                  </FormControl>
                  <FormDescription>
                    {t(
                      'When set to inherit, billing falls back to each selected channel.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {billingMode === 'fixed_request' && (
              <FormField
                control={form.control}
                name='fixed_request_credits'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Fixed request credits')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min='1'
                        step='1'
                        inputMode='numeric'
                        value={field.value ?? ''}
                        placeholder={t('Credits deducted per request')}
                        onChange={(e) =>
                          field.onChange(
                            e.target.value === '' ? 0 : Number(e.target.value)
                          )
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Each request deducts this many credits.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {billingMode === 'usage_tokens' && (
              <FormField
                control={form.control}
                name='token_billing_multiplier'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Token billing multiplier')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min='0'
                        step='0.1'
                        inputMode='decimal'
                        value={field.value ?? ''}
                        onChange={(e) =>
                          field.onChange(
                            e.target.value === '' ? 1 : Number(e.target.value)
                          )
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('1 means each upstream token deducts 1 credit.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {billingMode !== '' && (
              <FormField
                control={form.control}
                name='dynamic_billing_multiplier_enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between gap-3 rounded-lg border px-3 py-2.5'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-sm'>
                        {t('Enable dynamic billing multiplier')}
                      </FormLabel>
                      <FormDescription className='text-xs'>
                        {t(
                          'Apply a numeric multiplier returned by the upstream when present.'
                        )}
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
            )}

            <FormField
              control={form.control}
              name='channel_ids'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Member Channels')}</FormLabel>
                  <FormControl>
                    <MultiSelect
                      options={channels.map((c) => ({
                        label: c.name,
                        value: String(c.id),
                      }))}
                      selected={field.value.map((id) => String(id))}
                      onChange={(values) =>
                        field.onChange(
                          values
                            .map((v) => Number(v))
                            .filter((n) => Number.isFinite(n))
                        )
                      }
                      placeholder={t(
                        'Select member channels (empty default group means all channels)'
                      )}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Channels included in this group. Hidden from end users.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between gap-3 rounded-lg border px-3 py-2.5'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-sm'>
                      {field.value ? t('Enabled') : t('Disabled')}
                    </FormLabel>
                    <FormDescription className='text-xs'>
                      {isDefault
                        ? t('The default group is always enabled.')
                        : t('Disabled groups cannot be selected by API keys.')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      disabled={isDefault}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </form>
        </Form>
        <SheetFooter className='bg-background grid grid-cols-2 gap-2 border-t px-4 py-3 sm:flex sm:flex-row sm:justify-end sm:px-5 sm:py-4'>
          <SheetClose
            render={<Button variant='outline' className='w-full sm:w-auto' />}
          >
            {t('Close')}
          </SheetClose>
          <Button
            form='channel-group-form'
            type='submit'
            disabled={isSubmitting}
            className='w-full sm:w-auto'
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
