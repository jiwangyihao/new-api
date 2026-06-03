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
import { useMemo } from 'react'
import { z } from 'zod'
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  accountBalanceCentsToCnyAmount,
  accountBalanceCnyToCents,
} from '@/features/subscriptions/lib'
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
import { Switch } from '@/components/ui/switch'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({
  enabled: z.boolean(),
  minQuotaCny: z.string().refine((value) => {
    const amount = Number(value)
    return Number.isFinite(amount) && amount >= 0
  }),
  maxQuotaCny: z.string().refine((value) => {
    const amount = Number(value)
    return Number.isFinite(amount) && amount >= 0
  }),
})

type CheckinSettingsValues = {
  enabled: boolean
  minQuota: number
  maxQuota: number
}

type CheckinSettingsFormValues = z.infer<typeof schema>

type CheckinSettingsFormDefaults = CheckinSettingsFormValues

type OptionUpdate = { key: string; value: string }

function formatCnyAmountFromCents(cents: number): string {
  return accountBalanceCentsToCnyAmount(cents).toFixed(2)
}

function cnyInputToCentsString(value: string): string {
  return String(accountBalanceCnyToCents(Number(value)))
}

export function checkinSettingsToFormDefaults(
  values: CheckinSettingsValues
): CheckinSettingsFormDefaults {
  return {
    enabled: values.enabled,
    minQuotaCny: formatCnyAmountFromCents(values.minQuota),
    maxQuotaCny: formatCnyAmountFromCents(values.maxQuota),
  }
}

export function buildCheckinSettingsOptionUpdates(
  values: CheckinSettingsFormValues,
  initial: CheckinSettingsFormDefaults
): Array<OptionUpdate> {
  const updates: Array<OptionUpdate> = []

  if (values.enabled !== initial.enabled) {
    updates.push({
      key: 'checkin_setting.enabled',
      value: String(values.enabled),
    })
  }

  const minQuotaCents = cnyInputToCentsString(values.minQuotaCny)
  if (minQuotaCents !== cnyInputToCentsString(initial.minQuotaCny)) {
    updates.push({ key: 'checkin_setting.min_quota', value: minQuotaCents })
  }

  const maxQuotaCents = cnyInputToCentsString(values.maxQuotaCny)
  if (maxQuotaCents !== cnyInputToCentsString(initial.maxQuotaCny)) {
    updates.push({ key: 'checkin_setting.max_quota', value: maxQuotaCents })
  }

  return updates
}

export function CheckinSettingsSection({
  defaultValues,
}: {
  defaultValues: {
    enabled: boolean
    minQuota: number
    maxQuota: number
  }
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const formDefaults = useMemo<CheckinSettingsFormDefaults>(
    () => checkinSettingsToFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<CheckinSettingsFormValues>({
    resolver: zodResolver(schema) as unknown as Resolver<
      CheckinSettingsFormValues
    >,
    defaultValues: formDefaults,
  })

  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')

  async function onSubmit(values: CheckinSettingsFormValues) {
    const updates = buildCheckinSettingsOptionUpdates(values, formDefaults)

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }

    form.reset(values)
  }
  return (
    <SettingsSection
      title={t('Check-in Settings')}
      description={t('Configure daily check-in account balance rewards')}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          autoComplete='off'
          className='space-y-6'
        >
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                <div className='space-y-0.5'>
                  <FormLabel className='text-base'>
                    {t('Enable check-in feature')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Allow users to check in daily for random CNY account balance rewards'
                    )}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          {enabled && (
            <div className='grid gap-6 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='minQuotaCny'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Minimum check-in account balance reward (CNY)')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step='0.01'
                        placeholder={t('0.20')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Minimum CNY account balance credited for check-in')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='maxQuotaCny'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Maximum check-in account balance reward (CNY)')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step='0.01'
                        placeholder={t('1.50')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Maximum CNY account balance credited for check-in')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          )}

          <Button
            type='submit'
            disabled={!isDirty || updateOption.isPending || isSubmitting}
          >
            {updateOption.isPending || isSubmitting
              ? t('Saving...')
              : t('Save check-in settings')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
