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
import { useEffect, useState, type JSX } from 'react'
import * as z from 'zod'
import { useFieldArray, useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatTimestampToDate } from '@/lib/format'
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
import { searchUsers } from '@/features/users/api'
import type { User } from '@/features/users/types'
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
  excluded_at: z.coerce.number().int().nonnegative().optional(),
  excluded_by: z.coerce.number().int().nonnegative().optional(),
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
    const excludedAt =
      existing?.excluded_at ??
      (existingUsers.length === 0 ? item.excluded_at : undefined)
    const excludedBy =
      existing?.excluded_by ??
      (existingUsers.length === 0 ? item.excluded_by : undefined)
    return {
      user_id: item.user_id,
      ...(item.username ? { username: item.username } : {}),
      ...(item.reason ? { reason: item.reason } : {}),
      ...(excludedAt !== undefined ? { excluded_at: excludedAt } : {}),
      ...(excludedBy !== undefined ? { excluded_by: excludedBy } : {}),
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
export function formatSubscriptionAnalyticsExcludedAt(
  value: number | undefined
): string {
  if (value === undefined || value <= 0) return '—'
  return formatTimestampToDate(value)
}

export function formatSubscriptionAnalyticsExcludedBy(
  value: number | undefined
): string {
  if (value === undefined || value <= 0) return '—'
  return String(value)
}
function ExcludedUserSearchSelect(props: {
  disabled: boolean
  onSelect: (user: User) => void
}): JSX.Element {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(false)

  async function handleSearch() {
    const trimmed = keyword.trim()
    if (!trimmed) {
      setUsers([])
      return
    }
    setLoading(true)
    try {
      const response = await searchUsers({
        keyword: trimmed,
        p: 1,
        page_size: 8,
      })
      if (response.success) {
        setUsers(response.data?.items ?? [])
      } else {
        toast.error(response.message || t('Failed to search users'))
      }
    } catch {
      toast.error(t('Failed to search users'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className='space-y-2 md:col-span-2'>
      <FormLabel>
        {t('systemSettings.billing.statistics.searchUsers')}
      </FormLabel>
      <div className='flex gap-2'>
        <Input
          value={keyword}
          placeholder={t(
            'systemSettings.billing.statistics.searchUsersPlaceholder'
          )}
          disabled={props.disabled}
          onChange={(event) => setKeyword(event.target.value)}
          onKeyDown={(event) => {
            if (event.key !== 'Enter') return
            event.preventDefault()
            void handleSearch()
          }}
        />
        <Button
          type='button'
          variant='outline'
          disabled={props.disabled || loading}
          onClick={() => void handleSearch()}
        >
          {loading ? t('Loading...') : t('Search')}
        </Button>
      </div>
      {users.length > 0 ? (
        <div className='rounded-md border p-1'>
          {users.map((user) => (
            <Button
              key={user.id}
              type='button'
              variant='ghost'
              className='h-auto w-full justify-start px-2 py-1.5 text-left'
              disabled={props.disabled}
              onClick={() => props.onSelect(user)}
            >
              <span className='flex flex-col'>
                <span>{user.username || `#${user.id}`}</span>
                <span className='text-muted-foreground text-xs'>
                  #{user.id}
                  {user.display_name ? ` · ${user.display_name}` : ''}
                  {user.email ? ` · ${user.email}` : ''}
                </span>
              </span>
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  )
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
              excludedUsers.append({ user_id: 0, username: '', reason: '' })
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
          <ExcludedUserSearchSelect
            disabled={updateOption.isPending}
            onSelect={(user) =>
              excludedUsers.append({
                user_id: user.id,
                username: user.username,
                reason: '',
              })
            }
          />
          {excludedUsers.fields.length === 0 ? (
            <div className='text-muted-foreground rounded-md border p-3 text-sm'>
              {t('systemSettings.billing.statistics.noExcludedUsers')}
            </div>
          ) : null}
          {excludedUsers.fields.map((field, index) => (
            <div
              key={field.id}
              className='grid gap-3 rounded-md border p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_auto]'
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
              <div className='space-y-2'>
                <FormLabel>
                  {t('systemSettings.billing.statistics.excludedAt')}
                </FormLabel>
                <div className='text-muted-foreground rounded-md border px-3 py-2 text-sm'>
                  {formatSubscriptionAnalyticsExcludedAt(field.excluded_at)}
                </div>
              </div>
              <div className='space-y-2'>
                <FormLabel>
                  {t('systemSettings.billing.statistics.excludedBy')}
                </FormLabel>
                <div className='text-muted-foreground rounded-md border px-3 py-2 text-sm'>
                  {formatSubscriptionAnalyticsExcludedBy(field.excluded_by)}
                </div>
              </div>
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
