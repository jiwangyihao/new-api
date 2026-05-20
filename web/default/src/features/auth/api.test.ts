import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildRegisterRequestBody } from './api'

describe('register API payload', () => {
  test('maps affiliate code to backend aff_code and leaves Turnstile for query only', () => {
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
      aff_code: 'INVITE',
      trial_code: 'TRIAL-24H',
    })
  })

  test('keeps prefilled invite code in trial code field when it matches affiliate code', () => {
    const body = buildRegisterRequestBody({
      username: 'bob',
      password: 'password123',
      aff: 'INVITE42',
      trial_code: ' invite42 ',
    })

    assert.deepEqual(body, {
      username: 'bob',
      password: 'password123',
      aff_code: 'INVITE42',
      trial_code: 'invite42',
    })
  })

  test('keeps trial code as the invite candidate when no invite link was saved', () => {
    const body = buildRegisterRequestBody({
      username: 'carol',
      password: 'password123',
      trial_code: ' TrialOrInvite ',
    })

    assert.deepEqual(body, {
      username: 'carol',
      password: 'password123',
      trial_code: 'TrialOrInvite',
    })
  })
})
