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
import { useQueryClient } from '@tanstack/react-query'
import { useFieldArray, useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import * as z from 'zod'
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
  SettingsFormActionBar,
  SettingsFormSaveButton,
} from '../components/settings-form-actions'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { SubscriptionAnalyticsExcludedUser } from '../types'

const excludedUserSchema = z.object({
  user_id: z.coerce.number().int().positive(),
  username: z.string().trim().optional(),
  reason: z.string().trim().optional(),
})

const statisticsSettingsSchema = z.object({
  excludedUsers: z.array(excludedUserSchema),
})

type StatisticsSettingsValues = z.infer<typeof statisticsSettingsSchema>

type SubscriptionAnalyticsStatisticsSectionProps = {
  defaultValues: {
    excludedUsers: SubscriptionAnalyticsExcludedUser[]
  }
}

export function normalizeSubscriptionAnalyticsExcludedUsers(
  values: StatisticsSettingsValues,
  existingUsers: SubscriptionAnalyticsExcludedUser[] = []
): SubscriptionAnalyticsExcludedUser[] {
  const existingByUserID = new Map(
    existingUsers.map((item) => [item.user_id, item])
  )
  return values.excludedUsers.map((item) => {
    const existing = existingByUserID.get(item.user_id)
    return {
      user_id: item.user_id,
      ...(item.username ? { username: item.username } : {}),
      ...(item.reason ? { reason: item.reason } : {}),
      ...(existing?.excluded_at !== undefined
        ? { excluded_at: existing.excluded_at }
        : {}),
      ...(existing?.excluded_by !== undefined
        ? { excluded_by: existing.excluded_by }
        : {}),
    }
  })
}

export function buildSubscriptionAnalyticsExcludedUsersUpdate(
  excludedUsers: SubscriptionAnalyticsExcludedUser[]
) {
  return {
    key: 'subscription_analytics.excluded_users',
    value: JSON.stringify(excludedUsers),
  }
}

export function SubscriptionAnalyticsStatisticsSection(
  props: SubscriptionAnalyticsStatisticsSectionProps
) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const form = useForm<StatisticsSettingsValues>({
    resolver: zodResolver(statisticsSettingsSchema) as Resolver<
      StatisticsSettingsValues,
      unknown,
      StatisticsSettingsValues
    >,
    mode: 'onChange',
    defaultValues: { excludedUsers: props.defaultValues.excludedUsers },
  })
  const excludedUsers = useFieldArray({
    control: form.control,
    name: 'excludedUsers',
  })

  useEffect(() => {
    form.reset({ excludedUsers: props.defaultValues.excludedUsers })
  }, [form, props.defaultValues.excludedUsers])

  async function onSubmit(values: StatisticsSettingsValues) {
    const normalized = normalizeSubscriptionAnalyticsExcludedUsers(
      values,
      props.defaultValues.excludedUsers
    )
    const update = buildSubscriptionAnalyticsExcludedUsersUpdate(normalized)
    const response = await updateOption.mutateAsync(update)
    if (response.success) {
      await queryClient.invalidateQueries({ queryKey: ['admin-analytics'] })
      form.reset({ excludedUsers: normalized })
    } else {
      toast.error(response.message || t('Failed to update setting'))
    }
  }

  return (
    <SettingsSection
      title={t('systemSettings.billing.statistics.title')}
      description={t('systemSettings.billing.statistics.description')}
    >
      <Form {...form}>
        <SettingsFormActionBar>
          <Button
            type='button'
            variant='outline'
            onClick={() =>
              excludedUsers.append({ user_id: 1, username: '', reason: '' })
            }
            disabled={updateOption.isPending}
          >
            {t('systemSettings.billing.statistics.addExcludedUser')}
          </Button>
          <SettingsFormSaveButton
            form='subscription-analytics-statistics-form'
            isSaving={updateOption.isPending}
            idleLabel={t('systemSettings.billing.statistics.save')}
            savingLabel={t('Saving...')}
          />
        </SettingsFormActionBar>
        <form
          id='subscription-analytics-statistics-form'
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <FormDescription>
            {t('systemSettings.billing.statistics.help')}
          </FormDescription>
          {excludedUsers.fields.length === 0 ? (
            <div className='text-muted-foreground rounded-md border p-3 text-sm'>
              {t('systemSettings.billing.statistics.noExcludedUsers')}
            </div>
          ) : null}
          {excludedUsers.fields.map((field, index) => (
            <div
              key={field.id}
              className='grid gap-3 rounded-md border p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,2fr)_auto]'
            >
              <FormField
                control={form.control}
                name={`excludedUsers.${index}.user_id`}
                render={({ field: itemField }) => (
                  <FormItem>
                    <FormLabel>
                      {t('systemSettings.billing.statistics.userId')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        {...itemField}
                        onChange={(event) =>
                          itemField.onChange(Number(event.target.value))
                        }
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`excludedUsers.${index}.username`}
                render={({ field: itemField }) => (
                  <FormItem>
                    <FormLabel>
                      {t('systemSettings.billing.statistics.username')}
                    </FormLabel>
                    <FormControl>
                      <Input {...itemField} value={itemField.value ?? ''} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`excludedUsers.${index}.reason`}
                render={({ field: itemField }) => (
                  <FormItem>
                    <FormLabel>
                      {t('systemSettings.billing.statistics.reason')}
                    </FormLabel>
                    <FormControl>
                      <Input {...itemField} value={itemField.value ?? ''} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <div className='flex items-end'>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => excludedUsers.remove(index)}
                  disabled={updateOption.isPending}
                >
                  {t('systemSettings.billing.statistics.removeExcludedUser')}
                </Button>
              </div>
            </div>
          ))}
        </form>
      </Form>
    </SettingsSection>
  )
}
