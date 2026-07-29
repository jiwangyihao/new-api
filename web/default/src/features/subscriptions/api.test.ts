import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import { buildSubscriptionBalancePayRequestBody } from './api'

const apiSource = readFileSync(new URL('./api.ts', import.meta.url), 'utf8')

function exportedFunctionSource(name: string): string {
  const match = apiSource.match(
    new RegExp(`export async function ${name}\\([^]*?\\n}`)
  )
  assert.ok(match, `missing exported function ${name}`)
  return match[0]
}

describe('subscription balance payment API payload', () => {
  test('keeps the explicit purchase mode in request body', () => {
    assert.deepEqual(
      buildSubscriptionBalancePayRequestBody({
        plan_id: 42,
        purchase_mode: 'credit_balance',
        idempotency_key: 'balance-pay-1',
      }),
      {
        plan_id: 42,
        purchase_mode: 'credit_balance',
        idempotency_key: 'balance-pay-1',
      }
    )
  })
})

describe('subscription Kyren payment API helpers', () => {
  test('posts Kyren subscription payment requests to the subscription route', () => {
    const source = exportedFunctionSource('paySubscriptionKyren')

    assert.match(source, /\/api\/subscription\/kyren\/pay/)
    assert.match(source, /data/)
  })

  test('uses the admin subscription Kyren product status route', () => {
    const source = exportedFunctionSource('getSubscriptionKyrenProduct')

    assert.match(
      source,
      /`\/api\/subscription\/admin\/plans\/\$\{planId\}\/kyren\/product`/
    )
  })

  test('posts sync mode to the admin subscription Kyren product route', () => {
    const source = exportedFunctionSource('syncSubscriptionKyrenProduct')

    assert.match(
      source,
      /`\/api\/subscription\/admin\/plans\/\$\{planId\}\/kyren\/product`/
    )
    assert.match(source, /\{\s*mode\s*\}/)
  })
})

describe('home public plans API helper', () => {
  test('uses an isolated quiet public endpoint for the home page', () => {
    const source = exportedFunctionSource('getHomePublicPlansQuiet')

    assert.match(source, /\/api\/subscription\/public\/plans/)
    assert.match(source, /skipErrorHandler:\s*true/)
    assert.match(source, /skipBusinessError:\s*true/)
    assert.match(source, /disableDuplicate:\s*true/)
    assert.match(source, /catch\s*\{/)
    assert.match(source, /return\s*\{\s*success:\s*false,\s*data:\s*\[\]\s*\}/)
  })

  test('keeps the purchasable plans helper on the protected endpoint', () => {
    const source = exportedFunctionSource('getPublicPlans')

    assert.match(source, /\/api\/subscription\/plans/)
    assert.doesNotMatch(source, /\/api\/subscription\/public\/plans/)
  })
})
