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
  creemProductFromForm,
  creemProductToForm,
} from './creem-products-visual-editor'

describe('Creem products form helpers', () => {
  test('round trips account balance cents as CNY yuan', () => {
    assert.equal(creemProductToForm({ quota: 3990 }).balance_cny, '39.90')
    assert.equal(creemProductFromForm({ balance_cny: '39.90' }).quota, 3990)
  })

  test('keeps whole CNY account balance cents unchanged through a round trip', () => {
    const unchanged = creemProductFromForm(creemProductToForm({ quota: 4000 }))

    assert.equal(unchanged.quota, 4000)
  })
})
