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
import {
  cleanup,
  fireEvent,
  render,
  waitFor,
} from '@testing-library/react/pure'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, describe, test } from 'node:test'
import { I18nextProvider } from 'react-i18next'
import type { RedemptionMode, RedemptionResult } from '../types'
import { RedemptionDialog } from './redemption-dialog'

afterEach(cleanup)

async function createTestI18n() {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  return i18n
}

describe('redemption dialog', () => {
  test('requires keyboard-operable mode selection and submits explicit payload', async () => {
    const i18n = await createTestI18n()
    const submitted: Array<{
      key: string
      redemption_mode: RedemptionMode
    }> = []
    const view = render(
      <I18nextProvider i18n={i18n}>
        <RedemptionDialog
          open
          code='subscription-code'
          redeeming={false}
          onOpenChange={() => {}}
          onSubmit={async (request) => {
            submitted.push(request)
            return null
          }}
        />
      </I18nextProvider>
    )

    assert.ok(view.getByRole('dialog'))
    assert.ok(
      view.getByText('Keep the plan duration, reset cycle, and service limits.')
    )
    assert.ok(
      view.getByText(
        'Add the latest monthly Credit to the non-expiring balance; service limits come from the Credit balance plan.'
      )
    )
    const confirm = view.getByRole('button', { name: 'Confirm redemption' })
    assert.equal(confirm.hasAttribute('disabled'), true)

    const timed = view.getByRole('radio', { name: /Timed subscription/ })
    const credit = view.getByRole('radio', { name: /Credit balance/ })
    timed.focus()
    fireEvent.keyDown(timed, { key: 'ArrowDown', code: 'ArrowDown' })
    await waitFor(() =>
      assert.equal(credit.getAttribute('aria-checked'), 'true')
    )
    assert.equal(confirm.hasAttribute('disabled'), false)
    fireEvent.click(confirm)

    await waitFor(() => {
      assert.deepEqual(submitted, [
        { key: 'subscription-code', redemption_mode: 'credit_balance' },
      ])
    })
  })

  test('keeps the dialog state on error and shows a Credit receipt on success', async () => {
    const i18n = await createTestI18n()
    let attempt = 0
    const receipt: RedemptionResult = {
      type: 'subscription',
      quota: 0,
      redemption_mode: 'credit_balance',
      credit_balance: {
        user_subscription_id: 12,
        plan_id: 13,
        gross_credit: 1000,
        debt_offset: 300,
        available_credit: 700,
        settlement_debt: 0,
        balance_before: -300,
        balance_after: 700,
        active: true,
        ledger_id: 14,
        status: 'available',
      },
    }
    const view = render(
      <I18nextProvider i18n={i18n}>
        <RedemptionDialog
          open
          code='retry-code'
          redeeming={false}
          onOpenChange={() => {}}
          onSubmit={async () => {
            attempt += 1
            if (attempt === 1) {
              throw new Error('Credit balance redemption entry is closed')
            }
            return receipt
          }}
        />
      </I18nextProvider>
    )

    fireEvent.click(view.getByRole('radio', { name: /Credit balance/ }))
    fireEvent.click(view.getByRole('button', { name: 'Confirm redemption' }))
    await waitFor(() => view.getByRole('alert'))
    assert.match(view.getByRole('alert').textContent || '', /entry is closed/)
    assert.equal(
      view
        .getByRole('radio', { name: /Credit balance/ })
        .getAttribute('aria-checked'),
      'true'
    )

    fireEvent.click(view.getByRole('button', { name: 'Confirm redemption' }))
    await waitFor(() => view.getByText('Redemption receipt'))
    assert.match(
      view.getByRole('dialog').textContent || '',
      /Gross Credit.*1000/
    )
    assert.match(view.getByRole('dialog').textContent || '', /Debt offset.*300/)
    assert.match(
      view.getByRole('dialog').textContent || '',
      /Available Credit balance.*700/
    )
  })
})
