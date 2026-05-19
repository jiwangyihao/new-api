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
  test('keeps plan id and idempotency key in request body', () => {
    assert.deepEqual(
      buildSubscriptionBalancePayRequestBody({
        plan_id: 42,
        idempotency_key: 'balance-pay-1',
      }),
      {
        plan_id: 42,
        idempotency_key: 'balance-pay-1',
      }
    )
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
