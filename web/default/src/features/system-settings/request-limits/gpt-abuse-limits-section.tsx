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
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
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
import {
  SettingsFormActionBar,
  SettingsFormSaveButton,
} from '../components/settings-form-actions'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const gptAbuseLimitSchema = z.object({
  GPTAbuseLimitEnabled: z.boolean(),
  GPTAbuseDefaultWarningLimit: z.coerce.number().min(1).max(100000000),
})

export type GPTAbuseLimitOptionValues = z.infer<typeof gptAbuseLimitSchema>

export function collectGPTAbuseLimitUpdates(
  values: GPTAbuseLimitOptionValues,
  defaultValues: GPTAbuseLimitOptionValues
) {
  return Object.entries(values)
    .filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof GPTAbuseLimitOptionValues]
    )
    .map(([key, value]) => ({ key, value }))
}

type GPTAbuseLimitsSectionProps = {
  defaultValues: GPTAbuseLimitOptionValues
}

export function GPTAbuseLimitsSection(props: GPTAbuseLimitsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<GPTAbuseLimitOptionValues>({
    resolver: zodResolver(gptAbuseLimitSchema) as Resolver<
      GPTAbuseLimitOptionValues,
      unknown,
      GPTAbuseLimitOptionValues
    >,
    mode: 'onChange',
    defaultValues: props.defaultValues,
  })

  useEffect(() => {
    form.reset(props.defaultValues)
  }, [form, props.defaultValues])

  const onSubmit = async (values: GPTAbuseLimitOptionValues) => {
    const updates = collectGPTAbuseLimitUpdates(values, props.defaultValues)
    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection
      title={t('GPT abuse limits')}
      description={t(
        'Configure temporary GPT service interruption when upstream safety warnings reach the daily limit.'
      )}
    >
      <Form {...form}>
        <SettingsFormActionBar>
          <SettingsFormSaveButton
            form='gpt-abuse-limits-settings-form'
            isSaving={updateOption.isPending}
            idleLabel={t('Save GPT abuse limits')}
            savingLabel={t('Saving...')}
          />
        </SettingsFormActionBar>
        <form
          id='gpt-abuse-limits-settings-form'
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-6'
        >
          <FormField
            control={form.control}
            name='GPTAbuseLimitEnabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                <div className='space-y-0.5'>
                  <FormLabel className='text-base'>
                    {t('Enable GPT abuse service interruption')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, users who trigger the daily GPT safety warning limit are suspended from GPT service until the next day.'
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

          <FormField
            control={form.control}
            name='GPTAbuseDefaultWarningLimit'
            render={({ field }) => (
              <FormItem className='max-w-sm'>
                <FormLabel>{t('Default minimum GPT abuse warnings')}</FormLabel>
                <FormControl>
                  <div className='flex items-center gap-2'>
                    <Input
                      type='number'
                      min={1}
                      max={100000000}
                      step={1}
                      {...field}
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value, 10) || 1)
                      }
                    />
                    <span className='text-muted-foreground text-sm'>
                      {t('times')}
                    </span>
                  </div>
                </FormControl>
                <FormDescription>
                  {t(
                    'Plan limits and automatic concurrency-based limits are never allowed below this value.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </SettingsSection>
  )
}
