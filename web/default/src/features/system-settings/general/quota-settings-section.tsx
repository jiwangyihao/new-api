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
import { useMemo, type ChangeEvent } from 'react'
import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
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
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsFormActionBar,
  SettingsFormSaveButton,
} from '../components/settings-form-actions'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const quotaSchema = z.object({
  QuotaForNewUser: z.coerce.number().min(0),
  PreConsumedQuota: z.coerce.number().min(0),
  QuotaForInviter: z.coerce.number().min(0),
  QuotaForInvitee: z.coerce.number().min(0),
  TopUpLink: z.string(),
  general_setting: z.object({
    docs_link: z.string(),
  }),
  quota_setting: z.object({
    enable_free_model_pre_consume: z.boolean(),
  }),
})

export type QuotaFormValues = z.infer<typeof quotaSchema>

type OptionUpdate = { key: string; value: string | number | boolean }

function quotaSettingsToFormDefaults(values: QuotaFormValues): QuotaFormValues {
  return {
    ...values,
    QuotaForNewUser: accountBalanceCentsToCnyAmount(values.QuotaForNewUser),
    QuotaForInviter: accountBalanceCentsToCnyAmount(values.QuotaForInviter),
    QuotaForInvitee: accountBalanceCentsToCnyAmount(values.QuotaForInvitee),
  }
}

export function buildQuotaSettingsOptionUpdates(
  values: QuotaFormValues
): Array<OptionUpdate> {
  return [
    {
      key: 'QuotaForNewUser',
      value: String(accountBalanceCnyToCents(values.QuotaForNewUser)),
    },
    { key: 'PreConsumedQuota', value: values.PreConsumedQuota },
    {
      key: 'QuotaForInviter',
      value: String(accountBalanceCnyToCents(values.QuotaForInviter)),
    },
    {
      key: 'QuotaForInvitee',
      value: String(accountBalanceCnyToCents(values.QuotaForInvitee)),
    },
    { key: 'TopUpLink', value: values.TopUpLink },
    {
      key: 'general_setting.docs_link',
      value: values.general_setting.docs_link,
    },
    {
      key: 'quota_setting.enable_free_model_pre_consume',
      value: values.quota_setting.enable_free_model_pre_consume,
    },
  ]
}

type QuotaSettingsSectionProps = {
  defaultValues: QuotaFormValues
}

export function QuotaSettingsSection({
  defaultValues,
}: QuotaSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const handleNumberChange =
    (onChange: (value: number | string) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      onChange(
        event.target.value === '' ? '' : event.currentTarget.valueAsNumber
      )
    }

  const formDefaultValues = useMemo<QuotaFormValues>(
    () => quotaSettingsToFormDefaults(defaultValues),
    [defaultValues]
  )

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<QuotaFormValues>({
      resolver: zodResolver(quotaSchema) as Resolver<
        QuotaFormValues,
        unknown,
        QuotaFormValues
      >,
      defaultValues: formDefaultValues,
      onSubmit: async (data, changedFields) => {
        for (const update of buildQuotaSettingsOptionUpdates(data)) {
          if (
            !Object.prototype.hasOwnProperty.call(changedFields, update.key)
          ) {
            continue
          }

          await updateOption.mutateAsync(update)
        }
      },
    })

  return (
    <SettingsSection
      title={t('Quota Settings')}
      description={t(
        'Configure CNY account balance rewards and quota pre-consumption'
      )}
    >
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsFormActionBar>
          <SettingsFormSaveButton
            form='quota-settings-form'
            isSaving={updateOption.isPending || isSubmitting}
            idleLabel={t('Save Changes')}
            savingLabel={t('Saving...')}
          />
        </SettingsFormActionBar>
        <form
          id='quota-settings-form'
          onSubmit={handleSubmit}
          className='space-y-6'
        >
          <FormDirtyIndicator isDirty={isDirty} />
          <FormField
            control={form.control}
            name='QuotaForNewUser'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('New User Account Balance Reward (CNY)')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step='0.01'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('Initial CNY account balance credited to new users')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PreConsumedQuota'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Pre-Consumed Quota')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('Quota consumed before charging users')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='QuotaForInviter'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Inviter Account Balance Reward (CNY)')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step='0.01'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('CNY account balance credited to users who invite others')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='QuotaForInvitee'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Invitee Account Balance Reward (CNY)')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step='0.01'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('CNY account balance credited to invited users')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='quota_setting.enable_free_model_pre_consume'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                <div className='space-y-0.5'>
                  <FormLabel className='text-base'>
                    {t('Pre-Consume for Free Models')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, zero-cost models also pre-consume quota before final settlement.'
                    )}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='TopUpLink'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Top-Up Link')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('https://example.com/topup')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('External link for users to purchase quota')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='general_setting.docs_link'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Documentation Link')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('https://docs.example.com')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Link to your documentation site')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button
            type='submit'
            disabled={updateOption.isPending || isSubmitting}
          >
            {updateOption.isPending ? t('Saving...') : t('Save Changes')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
