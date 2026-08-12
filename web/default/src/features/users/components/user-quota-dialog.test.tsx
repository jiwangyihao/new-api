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
  accountBalanceAdjustmentInputToCents,
  formatUserAccountBalanceForDialog,
  getUserQuotaAdjustmentPreview,
} from './user-quota-dialog'

function readUserQuotaDialogSource(): string {
  return readFileSync(new URL('./user-quota-dialog.tsx', import.meta.url), 'utf8')
}

describe('UserQuotaDialog account balance helpers', () => {
  test('converts CNY account balance input to cents for submission', () => {
    assert.equal(accountBalanceAdjustmentInputToCents('40.00'), 4000)
    assert.equal(accountBalanceAdjustmentInputToCents('39.90'), 3990)
  })

  test('formats current account balance cents as CNY in the dialog', () => {
    assert.equal(formatUserAccountBalanceForDialog(4000), '¥40.00')
    assert.equal(formatUserAccountBalanceForDialog(3990), '¥39.90')
  })

  test('previews add adjustments using account balance CNY cents', () => {
    const preview = getUserQuotaAdjustmentPreview({
      currentBalanceCents: 4000,
      mode: 'add',
      input: '15.50',
    })

    assert.equal(preview.valueCents, 1550)
    assert.equal(preview.nextBalanceCents, 5550)
    assert.match(preview.text, /¥40\.00/)
    assert.match(preview.text, /\+¥15\.50/)
    assert.match(preview.text, /¥55\.50/)
  })

  test('previews subtract adjustments using account balance CNY cents', () => {
    const preview = getUserQuotaAdjustmentPreview({
      currentBalanceCents: 4000,
      mode: 'subtract',
      input: '15.50',
    })

    assert.equal(preview.valueCents, 1550)
    assert.equal(preview.nextBalanceCents, 2450)
    assert.match(preview.text, /¥40\.00/)
    assert.match(preview.text, /-¥15\.50/)
    assert.match(preview.text, /¥24\.50/)
  })

  test('previews override adjustments using account balance CNY cents', () => {
    const preview = getUserQuotaAdjustmentPreview({
      currentBalanceCents: 4000,
      mode: 'override',
      input: '15.50',
    })

    assert.equal(preview.valueCents, 1550)
    assert.equal(preview.nextBalanceCents, 1550)
    assert.match(preview.text, /¥40\.00/)
    assert.match(preview.text, /¥15\.50/)
  })
})

describe('UserQuotaDialog source contract', () => {
  test('uses task9 account balance helpers instead of legacy quota or currency formatting', () => {
    const source = readUserQuotaDialogSource()

    assert.match(source, /accountBalanceCnyToCents/)
    assert.match(source, /formatAccountBalanceForPlanPurchase/)
    assert.doesNotMatch(source, /\bgetCurrencyDisplay\b/)
    assert.doesNotMatch(source, /\bparseQuotaFromDollars\b/)
    assert.doesNotMatch(source, /\bformatQuota\b/)
  })
})
