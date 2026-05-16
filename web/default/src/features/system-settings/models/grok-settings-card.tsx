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
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  buildGrokFormDefaults,
  collectGrokSettingUpdates,
  grokSettingsFormSchema,
  type GrokSettingsFormValues,
  type GrokSettingsOptionValues,
} from './grok-settings-form'

const XAI_VIOLATION_FEE_DOC_URL =
  'https://docs.x.ai/docs/models#usage-guidelines-violation-fee'

interface Props {
  defaultValues: GrokSettingsOptionValues
}

export function GrokSettingsCard(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const formDefaults = useMemo(
    () => buildGrokFormDefaults(props.defaultValues),
    [props.defaultValues]
  )

  const form = useForm<GrokSettingsFormValues>({
    resolver: zodResolver(grokSettingsFormSchema) as Resolver<
      GrokSettingsFormValues,
      unknown,
      GrokSettingsFormValues
    >,
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const onSubmit = async (data: GrokSettingsFormValues) => {
    const updates = collectGrokSettingUpdates(data, props.defaultValues)

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const enabled = form.watch('grok.violation_deduction_enabled')

  return (
    <SettingsSection
      title={t('Grok Settings')}
      description={t('Configure xAI Grok model specific settings')}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
          <FormField
            control={form.control}
            name='grok.violation_deduction_enabled'
            render={({ field }) => (
              <FormItem className='flex items-center gap-2'>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
                <div>
                  <FormLabel>{t('Enable violation deduction')}</FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, violation requests will incur additional charges.'
                    )}{' '}
                    <a
                      href={XAI_VIOLATION_FEE_DOC_URL}
                      target='_blank'
                      rel='noreferrer'
                      className='underline'
                    >
                      {t('Official documentation')}
                    </a>
                  </FormDescription>
                </div>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='grok.violation_deduction_amount'
            render={({ field }) => (
              <FormItem className='max-w-xs'>
                <FormLabel>{t('Violation deduction amount')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    step={0.01}
                    min={0}
                    {...field}
                    disabled={!enabled}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Base amount. Actual deduction = base amount × system group rate.'
                  )}
                </FormDescription>
              </FormItem>
            )}
          />

          <Button type='submit' disabled={updateOption.isPending}>
            {updateOption.isPending ? t('Saving...') : t('Save Changes')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
