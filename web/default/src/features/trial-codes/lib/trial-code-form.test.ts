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
import { describe, test } from 'node:test'
import {
  formValuesToTrialCodePayload,
  getTrialCodeFormSchema,
  trialCodeToFormValues,
  type TrialCodeFormValues,
} from './trial-code-form'

const t = (key: string) => key

describe('trial code form mapping', () => {
  test('trims code and serializes expiration seconds', () => {
    const values: TrialCodeFormValues = {
      code: ' trial-24h ',
      plan_id: 3,
      enabled: false,
      max_redemptions: 10,
      expires_at: new Date('2026-05-15T00:00:00Z'),
    }

    assert.deepEqual(formValuesToTrialCodePayload(values), {
      code: 'trial-24h',
      plan_id: 3,
      enabled: false,
      max_redemptions: 10,
      expires_at: 1778803200,
    })
  })

  test('uses zero expiration for never expiring codes', () => {
    const values: TrialCodeFormValues = {
      code: 'TRIAL',
      plan_id: 1,
      enabled: true,
      max_redemptions: 0,
      expires_at: undefined,
    }

    assert.equal(formValuesToTrialCodePayload(values).expires_at, 0)
  })

  test('hydrates existing trial codes into form values', () => {
    const formValues = trialCodeToFormValues({
      id: 10,
      code: 'TRIAL-24H',
      plan_id: 2,
      enabled: true,
      max_redemptions: 20,
      redeemed_count: 7,
      expires_at: 1778803200,
      created_at: 1778716800,
      updated_at: 1778716800,
    })

    assert.equal(formValues.code, 'TRIAL-24H')
    assert.equal(formValues.plan_id, 2)
    assert.equal(formValues.enabled, true)
    assert.equal(formValues.max_redemptions, 20)
    assert.equal(formValues.expires_at?.toISOString(), '2026-05-15T00:00:00.000Z')
  })

  test('rejects missing plan id before submit', () => {
    const schema = getTrialCodeFormSchema(t)
    const result = schema.safeParse({
      code: 'TRIAL',
      plan_id: 0,
      enabled: true,
      max_redemptions: 0,
      expires_at: undefined,
    })

    assert.equal(result.success, false)
  })
})
