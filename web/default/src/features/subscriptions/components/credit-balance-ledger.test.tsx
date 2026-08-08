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
import userEvent from '@testing-library/user-event'
import { afterEach, describe, test } from 'bun:test'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { I18nextProvider } from 'react-i18next'
import type {
  CreditBalanceLedgerEntry,
  CreditBalanceLedgerFilters,
} from '../types'
import {
  CreditBalanceLedger,
  creditBalanceLedgerTypeLabel,
  formatCreditLedgerDelta,
  ledgerDateTimeToTimestamp,
} from './credit-balance-ledger'

const { cleanup, fireEvent, render, waitFor } =
  await import('@testing-library/react/pure')

afterEach(cleanup)

async function createTestI18n() {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: false,
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  return i18n
}

function ledgerEntry(
  id: number,
  type: 'refund' | 'chargeback',
  grossCredit: number
): CreditBalanceLedgerEntry {
  return {
    id,
    user_id: 7001,
    user_subscription_id: 7101,
    type,
    idempotency_key: `ledger-${id}`,
    source_type: 'subscription_order_recovery',
    source_id: 7201 + id,
    operation: type,
    terminal_state: type === 'refund' ? 'refunded' : 'chargeback',
    plan_id: 7401,
    gross_credit: grossCredit,
    net_credit: grossCredit,
    debt_offset: 0,
    debt_formed: Math.abs(grossCredit),
    consumed_available_credit: 400,
    settlement_debt_formed: 100,
    removed_exact_cost_micros: '12000000',
    removed_estimated_cost_micros: '3000000',
    removed_unknown_credit: 25,
    available_credit_before: 400,
    settlement_debt_before: 0,
    balance_before: 100,
    balance_after: 100 + grossCredit,
    available_credit_after: 0,
    settlement_debt_after: Math.abs(grossCredit),
    valuation_currency: 'CNY',
    valuation_rule_version: 1,
    valuation_state_version_after: 3,
    operator_user_id: 7301,
    payment_provider: 'stripe',
    reason: type === 'refund' ? 'provider refund' : 'provider dispute',
    created_at: 1_800_000_000 + id,
  }
}

describe('Credit balance ledger', () => {
  test('labels refund and chargeback distinctly and preserves negative deltas', () => {
    const t = ((key: string) => key) as Parameters<
      typeof creditBalanceLedgerTypeLabel
    >[1]
    assert.equal(creditBalanceLedgerTypeLabel('refund', t), 'Refund')
    assert.equal(creditBalanceLedgerTypeLabel('chargeback', t), 'Chargeback')
    assert.equal(formatCreditLedgerDelta(-500), '-500')
    assert.equal(formatCreditLedgerDelta(200), '+200')
  })

  test('applies date filters by keyboard and clears them', async () => {
    const i18n = await createTestI18n()
    const user = userEvent.setup()
    const calls: CreditBalanceLedgerFilters[] = []
    const loadEntries = async (filters: CreditBalanceLedgerFilters) => {
      calls.push(filters)
      return {
        success: true,
        data: [
          ledgerEntry(1, 'refund', -500),
          ledgerEntry(2, 'chargeback', -250),
        ],
      }
    }
    const view = render(
      <I18nextProvider i18n={i18n}>
        <CreditBalanceLedger loadEntries={loadEntries} />
      </I18nextProvider>
    )

    await waitFor(() => assert.equal(calls.length, 1))
    assert.ok(view.getByText('Refund'))
    assert.ok(view.getByText('Chargeback'))
    assert.ok(view.getByText('-500'))
    assert.ok(view.getByText('-250'))
    const refundRow = view.getByText('Refund').closest('tr')?.textContent || ''
    assert.match(refundRow, /Consumed available Credit.*400/)
    assert.match(refundRow, /Settlement debt formed.*100/)
    assert.match(refundRow, /Exact value removed.*12,000,000/)
    assert.match(refundRow, /Estimated value removed.*3,000,000/)
    assert.match(refundRow, /Unknown Credit removed.*25/)
    assert.match(refundRow, /Valuation currency.*CNY/)
    assert.match(refundRow, /Rule and state version.*1\/3/)
    assert.match(refundRow, /Terminal state.*refunded/)

    const sourceFilter = view.getByRole('combobox', {
      name: 'Ledger source filter',
    })
    sourceFilter.focus()
    await user.keyboard(
      '{Enter}{Home}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{Enter}'
    )
    assert.match(sourceFilter.textContent || '', /subscription_order_recovery/)

    const operationFilter = view.getByRole('combobox', {
      name: 'Ledger operation filter',
    })
    operationFilter.focus()
    await user.keyboard(
      '{Enter}{Home}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{Enter}'
    )
    assert.match(operationFilter.textContent || '', /chargeback/)

    const start = view.getByLabelText('Start time') as HTMLInputElement
    const end = view.getByLabelText('End time') as HTMLInputElement
    fireEvent.input(start, { target: { value: '2026-07-01T00:00' } })
    fireEvent.input(end, { target: { value: '2026-07-31T23:59' } })
    const apply = view.getByRole('button', { name: 'Apply filters' })
    apply.focus()
    assert.equal(document.activeElement, apply)
    await user.keyboard('{Enter}')

    await waitFor(() => assert.equal(calls.length, 2))
    assert.deepEqual(calls[1], {
      source_type: 'subscription_order_recovery',
      type: 'chargeback',
      start_time: ledgerDateTimeToTimestamp('2026-07-01T00:00'),
      end_time: ledgerDateTimeToTimestamp('2026-07-31T23:59'),
    })

    const clear = view.getByRole('button', { name: 'Clear filters' })
    clear.focus()
    await user.keyboard(' ')
    await waitFor(() => assert.equal(calls.length, 3))
    assert.deepEqual(calls[2], {})
    assert.equal(start.value, '')
    assert.equal(end.value, '')
    assert.match(
      view.getByRole('status').textContent || '',
      /2 ledger entries loaded/
    )
  })
})
