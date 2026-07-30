import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  RedemptionRequestError,
  submitInitialRedemption,
} from './use-redemption'

describe('initial redemption submission', () => {
  test('keeps the legacy wallet request mode-free', async () => {
    const requests: unknown[] = []
    let redeemed = false
    let modeRequired = false
    const errors: string[] = []

    await submitInitialRedemption({
      key: 'wallet-code',
      redeem: async (request) => {
        requests.push(request)
        return { type: 'wallet', quota: 100 }
      },
      onRedeemed: () => {
        redeemed = true
      },
      onModeRequired: () => {
        modeRequired = true
      },
      onError: (message) => errors.push(message),
      fallbackError: 'Redemption failed',
    })

    assert.deepEqual(requests, [{ key: 'wallet-code' }])
    assert.equal(redeemed, true)
    assert.equal(modeRequired, false)
    assert.deepEqual(errors, [])
  })

  test('reports an ordinary wallet redemption failure without opening mode selection', async () => {
    let modeRequired = false
    const errors: string[] = []

    await submitInitialRedemption({
      key: 'invalid-wallet-code',
      redeem: async () => {
        throw new RedemptionRequestError('Invalid redemption code')
      },
      onRedeemed: () => {
        assert.fail('failed redemption must not refresh success state')
      },
      onModeRequired: () => {
        modeRequired = true
      },
      onError: (message) => errors.push(message),
      fallbackError: 'Redemption failed',
    })

    assert.equal(modeRequired, false)
    assert.deepEqual(errors, ['Invalid redemption code'])
  })

  test('opens mode selection only for the structured mode-required error', async () => {
    let modeRequired = false
    const errors: string[] = []

    await submitInitialRedemption({
      key: 'subscription-code',
      redeem: async () => {
        throw new RedemptionRequestError(
          'Redemption mode is required',
          'redemption_mode_required'
        )
      },
      onRedeemed: () => {
        assert.fail('mode-required response is not a completed redemption')
      },
      onModeRequired: () => {
        modeRequired = true
      },
      onError: (message) => errors.push(message),
      fallbackError: 'Redemption failed',
    })

    assert.equal(modeRequired, true)
    assert.deepEqual(errors, [])
  })
})
