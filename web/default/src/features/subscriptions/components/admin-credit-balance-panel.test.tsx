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
import {
  fireEvent,
  render,
  waitFor,
  cleanup,
} from '@testing-library/react/pure'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, test } from 'bun:test'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { I18nextProvider } from 'react-i18next'
import { api } from '@/lib/api'
import type { PlanRecord } from '../types'
import {
  AdminCreditBalancePanel,
  creditBalanceAdjustmentErrorKey,
} from './admin-credit-balance-panel'

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
function afterSalesPlan(
  overrides: Partial<PlanRecord['plan']> = {}
): PlanRecord {
  return {
    plan: {
      id: 9101,
      title: '40 CNY / 1,000 Credit',
      price_amount: 40,
      price_amount_micros: '40000000',
      currency: 'CNY',
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'never',
      enabled: true,
      sort_order: 0,
      max_purchase_per_user: 0,
      total_amount: 0,
      monthly_token_limit: 1000,
      entitlement_type: 'timed',
      unlimited_purchase_enabled: true,
      channel_credit_equivalents: [],
      channel_token_equivalents: [],
      ...overrides,
    },
  }
}
function authoritativeAdjustmentResult(
  overrides: Record<string, unknown> = {}
) {
  return {
    plan_id: 9101,
    gross_credit: 800,
    net_credit: 800,
    gross_amount_micros: '32000000',
    net_amount_micros: '32000000',
    valuation_currency: 'CNY',
    source_currency: 'CNY',
    confidence: 'exact',
    fx_rate_numerator: '1',
    fx_rate_denominator: '1',
    fx_captured_at: 1_800_000_000,
    fx_direction: 'CNY->CNY',
    rule_version: 1,
    state_version_after: 1,
    consumed_available_credit: 0,
    debt_formed: 0,
    removed_exact_cost_micros: '0',
    removed_estimated_cost_micros: '0',
    removed_unknown_credit: 0,
    operation: 'increase',
    terminal_state: '',
    debt_offset: 0,
    available_credit: 800,
    settlement_debt: 0,
    balance_before: 0,
    balance_after: 800,
    replayed: false,
    preview: true,
    credit_balance: {
      user_subscription_id: 99,
      plan_id: 9101,
      gross_credit: 800,
      net_credit: 800,
      gross_amount_micros: '32000000',
      net_amount_micros: '32000000',
      valuation_currency: 'CNY',
      source_currency: 'CNY',
      valuation_confidence: 'exact',
      fx_rate_numerator: '1',
      fx_rate_denominator: '1',
      fx_captured_at: 1_800_000_000,
      fx_direction: 'CNY->CNY',
      valuation_rule_version: 1,
      valuation_state_version_after: 1,
      debt_offset: 0,
      available_credit: 800,
      settlement_debt: 0,
      balance_before: 0,
      balance_after: 800,
      active: true,
      ledger_id: 0,
      status: 'available',
    },
    ...overrides,
  }
}

describe('Admin Credit financial management', () => {
  test('keeps decrease errors separate from after-sales grant semantics', () => {
    const decreaseCodes = [
      'credit_valuation_plan_required',
      'credit_valuation_plan_ineligible',
      'credit_valuation_idempotency_mismatch',
      'credit_valuation_unsupported_currency',
      'unknown_error',
    ]

    for (const code of decreaseCodes) {
      assert.doesNotMatch(
        creditBalanceAdjustmentErrorKey(code, 'decrease'),
        /after-sales grant/i
      )
    }
    assert.match(
      creditBalanceAdjustmentErrorKey(
        'credit_valuation_idempotency_mismatch',
        'increase'
      ),
      /after-sales grant/i
    )
  })
  test('does not allow an increase without an eligible after-sales plan', async () => {
    const i18n = await createTestI18n()
    let adjustmentCalls = 0
    api.defaults.adapter = async (config) => {
      if (config.url?.endsWith('/credit-balance/ledger')) {
        return response(config, { success: true, data: [] })
      }
      if (config.url?.endsWith('/credit-balance/adjustments')) {
        adjustmentCalls += 1
        return response(config, { success: true, data: {} })
      }
      throw new Error(`unexpected request: ${config.method} ${config.url}`)
    }

    const view = render(
      <I18nextProvider i18n={i18n}>
        <AdminCreditBalancePanel userId={8101} />
      </I18nextProvider>
    )
    await waitFor(() => assert.ok(view.getByText('No Credit balance history')))

    fireEvent.change(view.getByLabelText('Credit amount'), {
      target: { value: '800' },
    })
    fireEvent.change(view.getAllByLabelText('Reason')[0], {
      target: { value: 'after-sales grant' },
    })
    const submit = view.getByRole('button', {
      name: 'Submit Credit adjustment',
    }) as HTMLButtonElement
    assert.equal(submit.disabled, true)
    fireEvent.click(submit)
    assert.equal(adjustmentCalls, 0)
  }, 20_000)
  test('shows the authoritative 32 CNY preview and clears it when amount changes', async () => {
    const i18n = await createTestI18n()
    const user = userEvent.setup()
    const previewPayloads: Array<Record<string, unknown>> = []
    api.defaults.adapter = async (config) => {
      if (config.url?.endsWith('/credit-balance/ledger')) {
        return response(config, { success: true, data: [] })
      }
      if (config.url?.endsWith('/credit-balance/adjustments/preview')) {
        previewPayloads.push(
          JSON.parse(String(config.data)) as Record<string, unknown>
        )
        return response(config, {
          success: true,
          data: authoritativeAdjustmentResult(),
        })
      }
      throw new Error(`unexpected request: ${config.method} ${config.url}`)
    }

    const view = render(
      <I18nextProvider i18n={i18n}>
        <AdminCreditBalancePanel userId={8101} plans={[afterSalesPlan()]} />
      </I18nextProvider>
    )
    await waitFor(() => assert.ok(view.getByText('No Credit balance history')))

    const plan = view.getByRole('combobox', {
      name: 'After-sales grant plan',
    })
    plan.focus()
    await user.keyboard('{Enter}{ArrowDown}{Enter}')
    const selectedPlanText =
      view.getByText('Selected after-sales grant plan').parentElement
        ?.textContent || ''
    assert.match(selectedPlanText, /¥40\.00/)
    assert.match(selectedPlanText, /Plan Credit:\s*1000/)
    assert.match(selectedPlanText, /Source currency:\s*CNY/)

    fireEvent.change(view.getByLabelText('Credit amount'), {
      target: { value: '800' },
    })
    fireEvent.click(
      view.getByRole('button', { name: 'Preview operational value' })
    )
    await waitFor(() =>
      assert.ok(view.getByText('Authoritative operational value preview'))
    )
    assert.deepEqual(previewPayloads, [
      { operation: 'increase', amount: '800', plan_id: 9101 },
    ])
    const previewText =
      view.getByText('Authoritative operational value preview').parentElement
        ?.textContent || ''
    assert.match(previewText, /Gross Credit:\s*800/)
    assert.match(previewText, /Net Credit:\s*800/)
    assert.match(previewText, /¥32\.00/)
    assert.match(previewText, /32,000,000\s*micros/)
    assert.match(previewText, /FX snapshot:\s*1\/1\s*CNY->CNY/)

    fireEvent.change(view.getByLabelText('Credit amount'), {
      target: { value: '801' },
    })
    assert.equal(
      view.queryByText('Authoritative operational value preview'),
      null
    )
  }, 20_000)
  test('keeps a failed retry key and replaces it after amount, plan, operation, and success changes', async () => {
    const i18n = await createTestI18n()
    const user = userEvent.setup()
    const adjustments: Array<Record<string, unknown>> = []
    let allowSuccess = false
    api.defaults.adapter = async (config) => {
      if (config.url?.endsWith('/credit-balance/ledger')) {
        return response(config, { success: true, data: [] })
      }
      if (config.url?.endsWith('/credit-balance/adjustments/preview')) {
        return response(config, {
          success: true,
          data: authoritativeAdjustmentResult({
            plan_id: Number(
              (JSON.parse(String(config.data)) as Record<string, unknown>)
                .plan_id
            ),
          }),
        })
      }
      if (config.url?.endsWith('/credit-balance/adjustments')) {
        const payload = JSON.parse(String(config.data)) as Record<
          string,
          unknown
        >
        adjustments.push(payload)
        if (!allowSuccess) {
          return response(config, {
            success: false,
            code: 'internal_error',
            message: 'unstable server text that the UI must ignore',
          })
        }
        const amount = Number(payload.amount)
        return response(config, {
          success: true,
          data: authoritativeAdjustmentResult({
            plan_id: Number(payload.plan_id ?? 0),
            gross_credit: amount,
            net_credit: amount,
            available_credit: amount,
            balance_after: amount,
            preview: false,
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
            debt_formed: 0,
          }),
        })
      }
      throw new Error(`unexpected request: ${config.method} ${config.url}`)
    }

    const secondPlan = afterSalesPlan({
      id: 9102,
      title: '50 CNY / 1,000 Credit',
      price_amount: 50,
      price_amount_micros: '50000000',
    })
    const view = render(
      <I18nextProvider i18n={i18n}>
        <AdminCreditBalancePanel
          userId={8101}
          plans={[afterSalesPlan(), secondPlan]}
        />
      </I18nextProvider>
    )
    await waitFor(() => assert.ok(view.getByText('No Credit balance history')))

    const plan = view.getByRole('combobox', {
      name: 'After-sales grant plan',
    })
    plan.focus()
    await user.keyboard('{Enter}{ArrowDown}{Enter}')
    const amount = view.getByLabelText('Credit amount')
    const reason = view.getAllByLabelText('Reason')[0]
    fireEvent.change(amount, { target: { value: '800' } })
    fireEvent.change(reason, { target: { value: 'controlled retry' } })
    const submit = view.getByRole('button', {
      name: 'Submit Credit adjustment',
    })

    fireEvent.click(submit)
    await waitFor(() => assert.equal(adjustments.length, 1))
    fireEvent.click(submit)
    await waitFor(() => assert.equal(adjustments.length, 2))
    assert.equal(adjustments[1].idempotency_key, adjustments[0].idempotency_key)
    fireEvent.change(amount, { target: { value: '801' } })
    fireEvent.click(submit)
    await waitFor(() => assert.equal(adjustments.length, 3))
    assert.equal(adjustments[2].amount, '801')
    assert.notEqual(
      adjustments[2].idempotency_key,
      adjustments[1].idempotency_key
    )

    fireEvent.click(
      view.getByRole('button', { name: 'Preview operational value' })
    )
    await waitFor(() =>
      assert.ok(view.getByText('Authoritative operational value preview'))
    )
    plan.focus()
    await user.keyboard('{Enter}{End}{Enter}')
    assert.equal(
      view.queryByText('Authoritative operational value preview'),
      null
    )
    fireEvent.click(submit)
    await waitFor(() => assert.equal(adjustments.length, 4))
    assert.equal(adjustments[3].plan_id, 9102)
    assert.notEqual(
      adjustments[3].idempotency_key,
      adjustments[2].idempotency_key
    )

    fireEvent.click(
      view.getByRole('button', { name: 'Preview operational value' })
    )
    await waitFor(() =>
      assert.ok(view.getByText('Authoritative operational value preview'))
    )
    const operation = view.getByRole('combobox', {
      name: 'Credit adjustment operation',
    })
    operation.focus()
    await user.keyboard('{Enter}{End}{Enter}')
    assert.equal(view.queryByLabelText('After-sales grant plan'), null)
    assert.equal(
      view.queryByText('Authoritative operational value preview'),
      null
    )
    allowSuccess = true
    fireEvent.click(submit)
    await waitFor(() => assert.equal(adjustments.length, 5))
    assert.equal(adjustments[4].operation, 'decrease')
    assert.equal('plan_id' in adjustments[4], false)
    assert.notEqual(
      adjustments[4].idempotency_key,
      adjustments[3].idempotency_key
    )

    fireEvent.change(amount, { target: { value: '801' } })
    fireEvent.change(reason, { target: { value: 'controlled retry' } })
    fireEvent.click(submit)
    await waitFor(() => assert.equal(adjustments.length, 6))
    assert.notEqual(
      adjustments[5].idempotency_key,
      adjustments[4].idempotency_key
    )
  }, 30_000)

  test('submits plan-bound increases and plan-free decreases from the keyboard', async () => {
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
            plan_id: payload.plan_id ?? 0,
            gross_credit: amount,
            net_credit: amount,
            gross_amount_micros: '0',
            net_amount_micros: '0',
            valuation_currency: 'CNY',
            source_currency: 'CNY',
            confidence: 'exact',
            fx_rate_numerator: '1',
            fx_rate_denominator: '1',
            fx_captured_at: 1_800_000_000,
            fx_direction: 'CNY->CNY',
            rule_version: 1,
            state_version_after: adjustments.length,
            consumed_available_credit:
              payload.operation === 'decrease' ? 50 : 0,
            debt_formed: payload.operation === 'decrease' ? 25 : 0,
            removed_exact_cost_micros:
              payload.operation === 'decrease' ? '12000000' : '0',
            removed_estimated_cost_micros:
              payload.operation === 'decrease' ? '3000000' : '0',
            removed_unknown_credit: payload.operation === 'decrease' ? 5 : 0,
            operation: payload.operation,
            terminal_state:
              payload.operation === 'decrease' ? 'admin_decrease' : '',
            debt_offset: 0,
            available_credit: payload.operation === 'increase' ? amount : 0,
            settlement_debt: payload.operation === 'decrease' ? 25 : 0,
            balance_before: 0,
            balance_after: payload.operation === 'increase' ? amount : -amount,
            replayed: false,
            preview: false,
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
              plan_id: Number(payload.plan_id ?? 0),
              gross_credit: amount,
              debt_offset: 0,
              available_credit: payload.operation === 'increase' ? amount : 0,
              settlement_debt: payload.operation === 'decrease' ? 25 : 0,
              balance_before: 0,
              balance_after:
                payload.operation === 'increase' ? amount : -amount,
              active: true,
              ledger_id: adjustments.length,
              status: payload.operation === 'increase' ? 'available' : 'debt',
            },
          },
        })
      }
      throw new Error(`unexpected request: ${config.method} ${config.url}`)
    }

    const view = render(
      <I18nextProvider i18n={i18n}>
        <AdminCreditBalancePanel userId={8101} plans={[afterSalesPlan()]} />
      </I18nextProvider>
    )
    await waitFor(() => assert.ok(view.getByText('No Credit balance history')))

    const plan = view.getByRole('combobox', {
      name: 'After-sales grant plan',
    })
    plan.focus()
    await user.keyboard('{Enter}{ArrowDown}{Enter}')
    const amount = view.getByLabelText('Credit amount')
    const reason = view.getAllByLabelText('Reason')[0]
    await user.type(amount, '125')
    await user.type(reason, 'approved increase')
    const submit = view.getByRole('button', {
      name: 'Submit Credit adjustment',
    })
    submit.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => assert.equal(adjustments.length, 1))
    assert.equal(adjustments[0].operation, 'increase')
    assert.equal(adjustments[0].amount, '125')
    assert.equal(adjustments[0].plan_id, 9101)
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
    assert.equal(adjustments[1].amount, '75')
    assert.equal('plan_id' in adjustments[1], false)
    assert.equal(adjustments[1].reason, 'approved decrease')
    assert.notEqual(
      adjustments[1].idempotency_key,
      adjustments[0].idempotency_key
    )
    const decreaseResult = view
      .getAllByRole('status')
      .map((status) => status.textContent || '')
      .join(' ')
    assert.match(decreaseResult, /Credit decrease committed/)
    assert.match(decreaseResult, /Consumed available Credit.*50/)
    assert.match(decreaseResult, /Settlement debt formed.*25/)
    assert.match(decreaseResult, /Exact value removed.*12,000,000/)
    assert.match(decreaseResult, /Estimated value removed.*3,000,000/)
    assert.match(decreaseResult, /Unknown Credit removed.*5/)
    assert.doesNotMatch(decreaseResult, /after-sales grant/i)
  }, 20_000)

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
            consumed_available_credit: 750,
            removed_exact_cost_micros: '30000000',
            removed_estimated_cost_micros: '5000000',
            removed_unknown_credit: 25,
            valuation_currency: 'CNY',
            rule_version: 1,
            state_version_after: 3,
            operation: 'chargeback',
            terminal_state: 'chargeback',
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
    assert.match(statusText, /Consumed available Credit.*750/)
    assert.match(statusText, /Exact value removed.*30,000,000/)
    assert.match(statusText, /Estimated value removed.*5,000,000/)
    assert.match(statusText, /Unknown Credit removed.*25/)
    assert.match(statusText, /Valuation currency.*CNY/)
    assert.match(statusText, /Rule and state version.*1\/3/)
    assert.match(statusText, /Terminal state.*chargeback/)
  })
})
