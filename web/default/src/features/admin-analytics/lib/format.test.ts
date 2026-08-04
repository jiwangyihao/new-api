import assert from 'node:assert/strict'
import test from 'node:test'
import {
  formatAdminMoneyAmount,
  formatAdminMoneyBreakdown,
} from './format'

test('money breakdown empty array displays dash', () => {
  assert.equal(formatAdminMoneyBreakdown([]), '—')
})

test('money amount preserves zero and currency', () => {
  assert.equal(formatAdminMoneyAmount({ amount: 0, currency: 'CNY' }), '¥0.00')
  assert.equal(formatAdminMoneyAmount({ amount: 0, currency: 'USD' }), '$0.00')
})

test('money breakdown keeps currencies separate', () => {
  assert.equal(
    formatAdminMoneyBreakdown([
      { amount: 1, currency: 'CNY' },
      { amount: 2, currency: 'USD' },
    ]),
    '¥1.00, $2.00'
  )
})

test('missing money amount displays dash', () => {
  assert.equal(formatAdminMoneyAmount(null), '—')
  assert.equal(formatAdminMoneyAmount(undefined), '—')
})

test('money amount prefers exact micros over legacy float', () => {
  assert.equal(
    formatAdminMoneyAmount({
      amount: 999,
      amount_micros: '32000000',
      currency: 'CNY',
    }),
    '¥32.00'
  )
})

test('money amount formats micros beyond the safe integer range with BigInt', () => {
  assert.equal(
    formatAdminMoneyAmount({
      amount: 0,
      amount_micros: '9007199254740993000000',
      currency: 'USD',
    }),
    '$9007199254740993.00'
  )
})
