import assert from 'node:assert/strict'
import test from 'node:test'
import { optionalNumericSearchParam } from './search'

test('optional numeric search param accepts repeated string params', () => {
  assert.equal(optionalNumericSearchParam(['42', '99']), 42)
})

test('optional numeric search param rejects unsafe values', () => {
  assert.equal(optionalNumericSearchParam(['0']), undefined)
  assert.equal(optionalNumericSearchParam(['-1']), undefined)
  assert.equal(optionalNumericSearchParam(['bad']), undefined)
  assert.equal(
    optionalNumericSearchParam([String(Number.MAX_SAFE_INTEGER + 1)]),
    undefined
  )
})
