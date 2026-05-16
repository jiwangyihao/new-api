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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

const longFormSaveCases = [
  {
    file: 'src/features/system-settings/general/system-info-section.tsx',
    formId: 'system-info-settings-form',
  },
  {
    file: 'src/features/system-settings/maintenance/sidebar-modules-section.tsx',
    formId: 'sidebar-modules-settings-form',
  },
  {
    file: 'src/features/system-settings/request-limits/rate-limit-section.tsx',
    formId: 'rate-limit-settings-form',
  },
  {
    file: 'src/features/system-settings/request-limits/ssrf-section.tsx',
    formId: 'ssrf-settings-form',
  },
  {
    file: 'src/features/system-settings/auth/basic-auth-section.tsx',
    formId: 'basic-auth-settings-form',
  },
  {
    file: 'src/features/system-settings/auth/oauth-section.tsx',
    formId: 'oauth-settings-form',
  },
  {
    file: 'src/features/system-settings/auth/passkey-section.tsx',
    formId: 'passkey-settings-form',
  },
  {
    file: 'src/features/system-settings/general/quota-settings-section.tsx',
    formId: 'quota-settings-form',
  },
  {
    file: 'src/features/system-settings/general/pricing-section.tsx',
    formId: 'pricing-settings-form',
  },
  {
    file: 'src/features/system-settings/integrations/monitoring-settings-section.tsx',
    formId: 'monitoring-settings-form',
  },
  {
    file: 'src/features/system-settings/integrations/email-settings-section.tsx',
    formId: 'email-settings-form',
  },
  {
    file: 'src/features/system-settings/integrations/payment-settings-section.tsx',
    formId: 'payment-settings-form',
  },
  {
    file: 'src/features/system-settings/models/global-settings-card.tsx',
    formId: 'global-model-settings-form',
  },
  {
    file: 'src/features/system-settings/models/gemini-settings-card.tsx',
    formId: 'gemini-settings-form',
  },
  {
    file: 'src/features/system-settings/models/model-ratio-form.tsx',
    formId: 'model-ratio-settings-form',
  },
  {
    file: 'src/features/system-settings/models/model-pricing-sheet.tsx',
    formId: 'model-pricing-editor-form',
  },
  {
    file: 'src/features/system-settings/maintenance/performance-section.tsx',
    formId: 'performance-settings-form',
  },
] as const

function readSource(file: string): string {
  return readFileSync(file, 'utf8')
}

describe('system settings long form save actions', () => {
  test('long forms render a form-bound save action before the long form body', () => {
    for (const item of longFormSaveCases) {
      const source = readSource(item.file)
      const topSaveIndex = source.indexOf(`form='${item.formId}'`)
      const formIndex = source.indexOf(`id='${item.formId}'`)

      assert.notEqual(
        topSaveIndex,
        -1,
        `${item.file} should render a top save action bound to ${item.formId}`
      )
      assert.notEqual(
        formIndex,
        -1,
        `${item.file} should assign ${item.formId} to the form`
      )
      assert.ok(
        topSaveIndex < formIndex,
        `${item.file} should render the form-bound save action before the long form body`
      )
    }
  })
})
