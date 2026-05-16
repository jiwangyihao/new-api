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
import {
  buildGrokFormDefaults,
  collectGrokSettingUpdates,
  type GrokSettingsOptionValues,
} from './grok-settings-form'

function readSource(file: string): string {
  return readFileSync(file, 'utf8')
}

describe('system settings dotted form keys', () => {
  test('grok settings use nested form values and emit flat option updates', () => {
    const defaultValues: GrokSettingsOptionValues = {
      'grok.violation_deduction_enabled': true,
      'grok.violation_deduction_amount': 0.05,
    }

    const values = buildGrokFormDefaults(defaultValues)

    values.grok.violation_deduction_enabled = false
    values.grok.violation_deduction_amount = 0.1

    assert.deepEqual(collectGrokSettingUpdates(values, defaultValues), [
      {
        key: 'grok.violation_deduction_enabled',
        value: false,
      },
      {
        key: 'grok.violation_deduction_amount',
        value: 0.1,
      },
    ])
  })

  test('grok settings do not keep flat dotted keys in the form schema', () => {
    const source = readSource(
      'src/features/system-settings/models/grok-settings-card.tsx'
    )

    assert.equal(
      source.includes("'grok.violation_deduction_enabled': z.boolean()"),
      false,
      'grok form schema should not use flat dotted keys'
    )
    assert.equal(
      source.includes("'grok.violation_deduction_amount': z.coerce"),
      false,
      'grok form schema should not use flat dotted keys'
    )
  })
})
