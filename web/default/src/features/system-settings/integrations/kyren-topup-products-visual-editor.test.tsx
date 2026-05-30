import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  KYREN_TOPUP_PRODUCTS_CONFLICT_MESSAGE,
  getKyrenTopUpProductStatusAlerts,
  saveKyrenTopUpProductsState,
  syncKyrenTopUpProductState,
  validateKyrenTopUpProducts,
  type KyrenTopUpProductStatus,
  type KyrenTopUpProductsState,
} from './kyren-topup-products-visual-editor'
import type { KyrenTopUpProduct } from '../types'

const validProduct = (overrides: Partial<KyrenTopUpProduct> = {}): KyrenTopUpProduct => ({
  id: 'topup_cny_10',
  name: '10 CNY',
  description: 'Recharge 10 CNY',
  product_id: 'prod_topup_cny_10',
  amount: '10.00',
  currency: 'CNY',
  quota: 1000,
  enabled: true,
  ...overrides,
})

describe('validateKyrenTopUpProducts', () => {
  test('rejects duplicate kyren top-up product ids', () => {
    assert.throws(
      () =>
        validateKyrenTopUpProducts([
          validProduct({ id: 'topup_cny_10', name: '10 CNY' }),
          validProduct({ id: 'topup_cny_10', name: '10 CNY again' }),
        ]),
      /duplicate/i
    )
  })

  test('rejects invalid amount, currency, quota, and empty id', () => {
    assert.throws(
      () => validateKyrenTopUpProducts([validProduct({ amount: '0.009' })]),
      /amount/i
    )
    assert.throws(
      () => validateKyrenTopUpProducts([validProduct({ currency: 'USD' })]),
      /CNY/i
    )
    assert.throws(
      () => validateKyrenTopUpProducts([validProduct({ quota: 0 })]),
      /quota/i
    )
    assert.throws(
      () => validateKyrenTopUpProducts([validProduct({ id: '   ' })]),
      /id/i
    )
  })

  test('emits translatable validation message keys', () => {
    assert.throws(
      () => validateKyrenTopUpProducts([validProduct({ id: '   ' })]),
      /Kyren top-up product id is required/
    )
    assert.throws(
      () =>
        validateKyrenTopUpProducts([
          validProduct({ id: 'dup' }),
          validProduct({ id: 'dup' }),
        ]),
      /Duplicate Kyren top-up product ID/
    )
    assert.throws(
      () => validateKyrenTopUpProducts([validProduct({ currency: 'USD' })]),
      /Kyren top-up products only support CNY/
    )
    assert.throws(
      () => validateKyrenTopUpProducts([validProduct({ amount: '0' })]),
      /Amount must be at least 0.01 CNY/
    )
    assert.throws(
      () => validateKyrenTopUpProducts([validProduct({ quota: 0 })]),
      /Quota must be at least 1/
    )
  })
})

describe('Kyren top-up products editor API state helpers', () => {
  test('sync success overwrites local products and version from the server response', async () => {
    const original: KyrenTopUpProductsState = {
      products: [validProduct({ id: 'topup_cny_10', product_id: '' })],
      version: 'v1',
      statuses: {},
    }
    const syncedProduct = validProduct({
      id: 'topup_cny_10',
      product_id: 'prod_synced',
      amount: '20.00',
    })

    const result = await syncKyrenTopUpProductState(original, 'topup_cny_10', {
      request: async (productId, mode) => {
        assert.equal(productId, 'topup_cny_10')
        assert.equal(mode, 'create_or_update')
        return {
          products: [syncedProduct],
          version: 'v2',
          product_id: 'prod_synced',
          status: 'ACTIVE',
          price: '20.00',
          currency: 'CNY',
          synced: true,
        }
      },
    })

    assert.deepEqual(result.products, [syncedProduct])
    assert.equal(result.version, 'v2')
    assert.deepEqual(result.statuses.topup_cny_10, {
      product_id: 'prod_synced',
      status: 'ACTIVE',
      price: '20.00',
      currency: 'CNY',
      price_matches: true,
      currency_matches: true,
    })
  })

  test('409 save conflict notifies, refetches, and returns server state', async () => {
    let conflictMessage = ''
    let refetchCount = 0
    const refetchedProduct = validProduct({ amount: '30.00' })

    const result = await saveKyrenTopUpProductsState({
      products: [validProduct()],
      version: 'stale-version',
      request: async () => {
        const error = new Error('conflict') as Error & {
          response: { status: number }
        }
        error.response = { status: 409 }
        throw error
      },
      refetch: async () => {
        refetchCount += 1
        return { products: [refetchedProduct], version: 'fresh-version' }
      },
      notifyConflict: (message) => {
        conflictMessage = message
      },
    })

    assert.equal(conflictMessage, KYREN_TOPUP_PRODUCTS_CONFLICT_MESSAGE)
    assert.equal(refetchCount, 1)
    assert.equal(result.conflicted, true)
    assert.deepEqual(result.state.products, [refetchedProduct])
    assert.equal(result.state.version, 'fresh-version')
  })

  test('status mismatch rendering model highlights archived and mismatch states', () => {
    const status: KyrenTopUpProductStatus = {
      product_id: 'prod_archived',
      status: 'ARCHIVED',
      price: '9.00',
      currency: 'USD',
      price_matches: false,
      currency_matches: false,
    }

    assert.deepEqual(getKyrenTopUpProductStatusAlerts(status), [
      'Kyren top-up product is archived',
      'Kyren top-up product price mismatch',
      'Kyren top-up product currency mismatch',
    ])
  })
})
