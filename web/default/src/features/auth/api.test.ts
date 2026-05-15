import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildRegisterRequestBody } from './api'

describe('register API payload', () => {
  test('keeps trial code in JSON body and leaves Turnstile for query only', () => {
    const body = buildRegisterRequestBody({
      username: 'alice',
      password: 'password123',
      email: 'alice@example.com',
      verification_code: '123456',
      aff: 'INVITE',
      trial_code: 'TRIAL-24H',
      turnstile: 'turnstile-token',
    })

    assert.deepEqual(body, {
      username: 'alice',
      password: 'password123',
      email: 'alice@example.com',
      verification_code: '123456',
      aff: 'INVITE',
      trial_code: 'TRIAL-24H',
    })
  })
})
