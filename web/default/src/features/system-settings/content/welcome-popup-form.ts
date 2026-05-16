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
import * as z from 'zod'
import type {
  ContentSettings,
  UpdateOptionRequest,
  WelcomePopupFrequency,
} from '../types'

export type WelcomePopupOptionValues = Pick<
  ContentSettings,
  | 'console_setting.welcome_popup_content'
  | 'console_setting.welcome_popup_enabled'
  | 'console_setting.welcome_popup_frequency'
>

export type WelcomePopupFormValues = {
  content: string
  frequency: WelcomePopupFrequency
}

type WelcomePopupField = {
  key:
    | 'console_setting.welcome_popup_content'
    | 'console_setting.welcome_popup_frequency'
  getValue: (values: WelcomePopupFormValues) => UpdateOptionRequest['value']
}

const WELCOME_POPUP_FIELDS = [
  {
    key: 'console_setting.welcome_popup_content',
    getValue: (values) => values.content,
  },
  {
    key: 'console_setting.welcome_popup_frequency',
    getValue: (values) => values.frequency,
  },
] satisfies readonly WelcomePopupField[]

const welcomePopupFrequencySchema = z.enum([
  'once_per_version',
  'once_per_day',
  'every_session',
])

type Translate = (key: string) => string

export function countUnicodeCharacters(value: string): number {
  return Array.from(value).length
}

export function createWelcomePopupFormSchema(t: Translate) {
  return z.object({
    content: z.string().refine(
      (value) => countUnicodeCharacters(value) <= 2000,
      {
        message: t('Welcome popup content must be at most 2000 characters.'),
      }
    ),
    frequency: welcomePopupFrequencySchema,
  })
}

export const welcomePopupFormSchema = createWelcomePopupFormSchema((key) => key)

export function buildWelcomePopupFormDefaults(
  options: WelcomePopupOptionValues
): WelcomePopupFormValues {
  return {
    content: options['console_setting.welcome_popup_content'] ?? '',
    frequency:
      options['console_setting.welcome_popup_frequency'] ?? 'once_per_version',
  }
}

export function collectWelcomePopupSettingUpdates(
  values: WelcomePopupFormValues,
  defaults: WelcomePopupOptionValues
): UpdateOptionRequest[] {
  const updates: UpdateOptionRequest[] = []

  for (const field of WELCOME_POPUP_FIELDS) {
    const value = field.getValue(values)
    if (value !== defaults[field.key]) {
      updates.push({ key: field.key, value })
    }
  }

  return updates
}
