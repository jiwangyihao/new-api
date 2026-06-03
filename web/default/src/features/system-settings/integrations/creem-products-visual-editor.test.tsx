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
