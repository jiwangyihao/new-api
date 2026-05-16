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
import { useEffect, useMemo, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import type { Resolver } from 'react-hook-form'
import { useForm } from 'react-hook-form'
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
import { SafeMarkdown } from '@/components/ui/safe-markdown'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  buildWelcomePopupFormDefaults,
  collectWelcomePopupSettingUpdates,
  createWelcomePopupFormSchema,
  type WelcomePopupFormValues,
  type WelcomePopupOptionValues,
} from './welcome-popup-form'

const frequencyOptions = [
  {
    value: 'once_per_version',
    labelKey: 'Show once per content update',
  },
  {
    value: 'once_per_day',
    labelKey: 'Show once per day',
  },
  {
    value: 'every_session',
    labelKey: 'Show every system session',
  },
] as const

type WelcomePopupSectionProps = {
  defaultValues: WelcomePopupOptionValues
}

export function WelcomePopupSection(props: WelcomePopupSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const defaultEnabled =
    props.defaultValues['console_setting.welcome_popup_enabled']
  const [enabled, setEnabled] = useState(defaultEnabled)
  const formDefaults = useMemo(
    () => buildWelcomePopupFormDefaults(props.defaultValues),
    [props.defaultValues]
  )
  const formSchema = useMemo(() => createWelcomePopupFormSchema(t), [t])

  const form = useForm<WelcomePopupFormValues>({
    resolver: zodResolver(formSchema) as Resolver<
      WelcomePopupFormValues,
      unknown,
      WelcomePopupFormValues
    >,
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  useEffect(() => {
    setEnabled(defaultEnabled)
  }, [defaultEnabled])

  const handleEnabledChange = async (checked: boolean) => {
    try {
      const response = await updateOption.mutateAsync({
        key: 'console_setting.welcome_popup_enabled',
        value: checked,
      })

      if (response.success) {
        setEnabled(checked)
      }
    } catch {
      // useUpdateOption reports the mutation error.
    }
  }

  const onSubmit = async (values: WelcomePopupFormValues) => {
    const updates = collectWelcomePopupSettingUpdates(
      values,
      props.defaultValues
    )

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      const response = await updateOption.mutateAsync(update)
      if (!response.success) return
    }

  }

  const content = form.watch('content')

  return (
    <SettingsSection
      title={t('Welcome Popup')}
      description={t(
        'This popup appears after users enter the authenticated system area.'
      )}
    >
      <div className='space-y-6'>
        <div className='flex items-center justify-between gap-4 rounded-lg border p-4'>
          <div className='space-y-0.5'>
            <div className='font-medium'>{t('Enabled')}</div>
            <p className='text-muted-foreground text-sm'>
              {t('This content is returned only to authenticated users.')}
            </p>
          </div>
          <Switch
            checked={enabled}
            disabled={updateOption.isPending}
            onCheckedChange={handleEnabledChange}
          />
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
            <FormField
              control={form.control}
              name='content'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Welcome announcement content')}</FormLabel>
                  <FormControl>
                    <Textarea rows={8} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Markdown is supported. Raw HTML is not supported.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='frequency'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Display frequency')}</FormLabel>
                  <Select
                    items={[
                      ...frequencyOptions.map((option) => ({
                        value: option.value,
                        label: t(option.labelKey),
                      })),
                    ]}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {frequencyOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {t(option.labelKey)}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='rounded-lg border p-4'>
              <div className='mb-3 text-sm font-medium'>{t('Preview')}</div>
              <SafeMarkdown>{content}</SafeMarkdown>
            </div>

            <Button type='submit' disabled={updateOption.isPending}>
              {updateOption.isPending ? t('Saving...') : t('Save welcome popup')}
            </Button>
          </form>
        </Form>
      </div>
    </SettingsSection>
  )
}
