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
import { test } from 'node:test'
import {
  formatCredits,
  getCreditSettlement,
  isCreditBillingLog,
} from '@/lib/credits'

test('recognizes only explicitly marked credit logs', () => {
  assert.equal(isCreditBillingLog({ billing_unit: 'credit' }), true)
  assert.equal(isCreditBillingLog({ billing_unit: 'legacy_quota' }), false)
  assert.equal(isCreditBillingLog({}), false)
})

test('formats credits as integer units without currency conversion', () => {
  assert.equal(formatCredits(160000, 'en-US'), '160,000 Credit')
  assert.equal(formatCredits(160000, 'zh-CN'), '160,000 Credit')
})

test('describes negative settlement delta as released pre-consumption', () => {
  assert.deepEqual(getCreditSettlement(-80), {
    kind: 'released',
    credits: 80,
  })
})

test('describes positive and zero settlement deltas without signed amounts', () => {
  assert.deepEqual(getCreditSettlement(20), {
    kind: 'charged',
    credits: 20,
  })
  assert.deepEqual(getCreditSettlement(0), { kind: 'none', credits: 0 })
})
