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
import type { InternalAxiosRequestConfig } from 'axios'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, test } from 'bun:test'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { I18nextProvider } from 'react-i18next'
import { api } from '@/lib/api'
import { AdminCreditBalancePanel } from './admin-credit-balance-panel'

const { cleanup, render, waitFor } = await import('@testing-library/react/pure')
const originalAPIAdapter = api.defaults.adapter

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAPIAdapter
})

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

function response(config: InternalAxiosRequestConfig, data: unknown) {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

describe('Admin Credit financial management', () => {
  test('submits structured increase and decrease adjustments from the keyboard', async () => {
    const i18n = await createTestI18n()
    const user = userEvent.setup()
    const adjustments: Array<Record<string, unknown>> = []
    api.defaults.adapter = async (config) => {
      if (config.url?.endsWith('/credit-balance/ledger')) {
        return response(config, { success: true, data: [] })
      }
      if (config.url?.endsWith('/credit-balance/adjustments')) {
        const payload = JSON.parse(String(config.data)) as Record<
          string,
          unknown
        >
        adjustments.push(payload)
        const amount = Number(payload.amount)
        return response(config, {
          success: true,
          data: {
            adjustment: {
              id: adjustments.length,
              idempotency_key: payload.idempotency_key,
              user_id: 8101,
              operation: payload.operation,
              amount,
              operator_user_id: 1,
              reason: payload.reason,
              ledger_id: adjustments.length,
              created_at: 1_800_000_000,
            },
            credit_balance: {
              user_subscription_id: 99,
              plan_id: 98,
              gross_credit: payload.operation === 'increase' ? amount : -amount,
              debt_offset: 0,
              available_credit: payload.operation === 'increase' ? amount : 0,
              settlement_debt: payload.operation === 'decrease' ? amount : 0,
              balance_before: 0,
              balance_after:
                payload.operation === 'increase' ? amount : -amount,
              active: true,
              ledger_id: adjustments.length,
              status: payload.operation === 'increase' ? 'available' : 'debt',
            },
            debt_formed: payload.operation === 'decrease' ? amount : 0,
            replayed: false,
          },
        })
      }
      throw new Error(`unexpected request: ${config.method} ${config.url}`)
    }

    const view = render(
      <I18nextProvider i18n={i18n}>
        <AdminCreditBalancePanel userId={8101} />
      </I18nextProvider>
    )
    await waitFor(() => assert.ok(view.getByText('No Credit balance history')))

    const amount = view.getByLabelText('Credit amount')
    const reason = view.getAllByLabelText('Reason')[0]
    await user.type(amount, '125')
    await user.type(reason, 'approved increase')
    const submit = view.getByRole('button', {
      name: 'Submit Credit adjustment',
    })
    submit.focus()
    assert.equal(document.activeElement, submit)
    await user.keyboard('{Enter}')
    await waitFor(() => assert.equal(adjustments.length, 1))
    assert.equal(adjustments[0].operation, 'increase')
    assert.equal(adjustments[0].amount, 125)
    assert.equal(adjustments[0].reason, 'approved increase')
    assert.match(String(adjustments[0].idempotency_key), /^admin-credit-8101-/)

    const operation = view.getByRole('combobox', {
      name: 'Credit adjustment operation',
    })
    operation.focus()
    await user.keyboard('{Enter}{End}{Enter}')
    await user.type(amount, '75')
    await user.type(reason, 'approved decrease')
    submit.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => assert.equal(adjustments.length, 2))
    assert.equal(adjustments[1].operation, 'decrease')
    assert.equal(adjustments[1].amount, 75)
    assert.equal(adjustments[1].reason, 'approved decrease')
    assert.notEqual(
      adjustments[1].idempotency_key,
      adjustments[0].idempotency_key
    )
    assert.ok(
      view
        .getAllByRole('status')
        .some((status) => /debt: 75/.test(status.textContent || ''))
    )
  })

  test('requires preview and submits a verified chargeback terminal by keyboard', async () => {
    const i18n = await createTestI18n()
    const user = userEvent.setup()
    const recoveryCalls: Array<Record<string, unknown>> = []
    api.defaults.adapter = async (config) => {
      if (config.url?.endsWith('/credit-balance/ledger')) {
        return response(config, { success: true, data: [] })
      }
      if (config.url?.endsWith('/recovery-preview')) {
        return response(config, {
          success: true,
          data: {
            order_id: 91,
            user_id: 8101,
            username: 'verified-user',
            plan_id: 92,
            plan_title: 'Verified Credit order',
            trade_no: 'trade verified/1',
            money: 40,
            amount_cents: 4000,
            currency: 'CNY',
            payment_provider: 'stripe',
            payment_method: 'stripe',
            purchase_mode: 'credit_balance',
            status: 'success',
            complete_time: 1_800_000_000,
            gross_credit: 1000,
          },
        })
      }
      if (config.url?.endsWith('/recovery')) {
        const payload = JSON.parse(String(config.data)) as Record<
          string,
          unknown
        >
        recoveryCalls.push(payload)
        return response(config, {
          success: true,
          data: {
            order_id: 91,
            trade_no: 'trade verified/1',
            status: 'chargeback',
            recovery_type: 'chargeback',
            gross_credit: 1000,
            debt_formed: 250,
            available_credit: 0,
            settlement_debt: 250,
            balance_before: 750,
            balance_after: -250,
            ledger_id: 93,
            replayed: false,
          },
        })
      }
      throw new Error(`unexpected request: ${config.method} ${config.url}`)
    }

    const view = render(
      <I18nextProvider i18n={i18n}>
        <AdminCreditBalancePanel userId={8101} />
      </I18nextProvider>
    )
    await waitFor(() => assert.ok(view.getByText('No Credit balance history')))
    const confirm = view.getByRole('button', {
      name: 'Confirm financial terminal',
    })
    assert.equal((confirm as HTMLButtonElement).disabled, true)

    const tradeNo = view.getByLabelText('Order number')
    await user.type(tradeNo, 'trade verified/1')
    const preview = view.getByRole('button', { name: 'Preview order' })
    preview.focus()
    await user.keyboard(' ')
    await waitFor(() =>
      assert.ok(view.getByText('Verify order ownership and amount'))
    )
    const previewText =
      view.getByText('Verify order ownership and amount').parentElement
        ?.textContent || ''
    assert.match(previewText, /verified-user/)
    assert.match(previewText, /40 CNY/)
    assert.match(previewText, /1000/)
    assert.match(previewText, /stripe/)
    assert.equal((confirm as HTMLButtonElement).disabled, false)

    const terminal = view.getByRole('combobox', { name: 'Financial terminal' })
    terminal.focus()
    await user.keyboard('{Enter}{End}{Enter}')
    const recoveryReason = view.getAllByLabelText('Reason')[1]
    await user.type(recoveryReason, 'verified provider dispute')
    confirm.focus()
    await user.keyboard('{Enter}')

    await waitFor(() => assert.equal(recoveryCalls.length, 1))
    assert.deepEqual(recoveryCalls[0], {
      recovery_type: 'chargeback',
      reason: 'verified provider dispute',
    })
    const statusText = view
      .getAllByRole('status')
      .map((status) => status.textContent || '')
      .join(' ')
    assert.match(statusText, /chargeback/)
    assert.match(statusText, /1000/)
    assert.match(statusText, /250/)
  })
})
