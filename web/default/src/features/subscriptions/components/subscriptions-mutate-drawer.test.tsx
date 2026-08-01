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
import userEvent from '@testing-library/user-event'
import { afterEach, describe, test } from 'bun:test'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { I18nextProvider } from 'react-i18next'
import { api } from '@/lib/api'
import type { PlanPayload, PlanRecord } from '../types'
import { SubscriptionsMutateDrawer } from './subscriptions-mutate-drawer'
import { SubscriptionsProvider } from './subscriptions-provider'

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

const riskyPlan: PlanRecord = {
  existing_timed_entitlement_count: 3,
  plan: {
    id: 9101,
    title: 'Monthly plan',
    subtitle: '',
    price_amount: 40,
    currency: 'CNY',
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'monthly',
    enabled: true,
    sort_order: 0,
    max_purchase_per_user: 0,
    total_amount: 0,
    monthly_token_limit: 100,
    concurrency_limit: 1,
    queue_capacity: 0,
    gpt_abuse_warning_limit: 0,
    is_trial: false,
    invite_trial: false,
    public_visible: true,
    trial_duration_hours: 0,
    reward_eligible: true,
    business_code: 'monthly-plan',
    entitlement_type: 'timed',
    unlimited_purchase_enabled: false,
    timed_conversion_enabled: true,
  },
}

async function renderRiskDrawer() {
  const i18n = await createTestI18n()
  const user = userEvent.setup()
  const submissions: PlanPayload[] = []
  api.defaults.adapter = async (config) => {
    if (config.method === 'get' && config.url?.endsWith('/kyren/product')) {
      return {
        data: { success: true, data: null },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    if (config.method === 'put' && config.url?.endsWith('/admin/plans/9101')) {
      submissions.push(JSON.parse(String(config.data)) as PlanPayload)
      return {
        data: { success: true, data: riskyPlan },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    throw new Error(`unexpected request: ${config.method} ${config.url}`)
  }

  const view = render(
    <I18nextProvider i18n={i18n}>
      <SubscriptionsProvider>
        <SubscriptionsMutateDrawer
          open
          onOpenChange={() => undefined}
          currentRow={riskyPlan}
        />
      </SubscriptionsProvider>
    </I18nextProvider>
  )
  const monthlyCredit = await view.findByLabelText('Monthly Credits')
  await user.clear(monthlyCredit)
  await user.type(monthlyCredit, '200')
  assert.ok(view.getByText('Monthly Credit change risk'))
  assert.match(
    view.getByText(
      /This plan has 3 active or conversion-grace timed entitlements/
    ).textContent || '',
    /refund basis/
  )
  return {
    view,
    user,
    submissions,
    monthlyCredit,
    checkbox: view.getByRole('checkbox', {
      name: 'I accept the renewal-merging risk',
    }),
    reason: view.getByLabelText('Risk confirmation reason'),
    save: view.getByRole('button', { name: 'Save changes' }),
    form: document.getElementById('subscription-form') as HTMLFormElement,
  }
}

async function flushSubmission(): Promise<void> {
  await new Promise<void>((resolve) => setTimeout(resolve, 0))
}

function bridgeExternalSubmitButton(save: HTMLElement, form: HTMLFormElement) {
  let activations = 0
  save.addEventListener('click', (event) => {
    // happy-dom does not implement an out-of-tree submit button's `form`
    // association. Preserve the real keyboard click and bridge only that
    // missing platform behavior into React's native submit handler.
    event.preventDefault()
    activations += 1
    fireEvent.submit(form)
  })
  return () => activations
}

describe('subscription monthly Credit risk confirmation', () => {
  test('rejects keyboard submission when confirmation is missing', async () => {
    const { user, submissions, reason, save, form } = await renderRiskDrawer()
    const activationCount = bridgeExternalSubmitButton(save, form)
    await user.type(reason, 'reason without confirmation')
    save.focus()
    await user.keyboard('{Enter}')
    await flushSubmission()
    assert.equal(activationCount(), 1)
    assert.equal(submissions.length, 0)
  })

  test('rejects keyboard submission when the confirmation reason is empty', async () => {
    const { user, submissions, checkbox, save, form } = await renderRiskDrawer()
    const activationCount = bridgeExternalSubmitButton(save, form)
    checkbox.focus()
    await user.keyboard(' ')
    assert.equal(checkbox.getAttribute('aria-checked'), 'true')
    save.focus()
    await user.keyboard('{Enter}')
    await flushSubmission()
    assert.equal(activationCount(), 1)
    assert.equal(submissions.length, 0)
  })

  test('submits exactly once after Space, Tab, and Enter complete the risk contract', async () => {
    const { user, submissions, checkbox, reason, save, form } =
      await renderRiskDrawer()
    const activationCount = bridgeExternalSubmitButton(save, form)
    assert.equal(save.getAttribute('form'), 'subscription-form')
    assert.equal(save.getAttribute('type'), 'submit')
    checkbox.focus()
    assert.equal(document.activeElement, checkbox)
    assert.match(checkbox.className, /focus-visible:ring/)
    await user.keyboard(' ')
    assert.equal(checkbox.getAttribute('aria-checked'), 'true')
    await user.tab()
    assert.equal(document.activeElement, reason)
    await user.type(reason, 'approved after reviewing renewal cohorts')

    save.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => assert.equal(submissions.length, 1))
    assert.equal(activationCount(), 1)
    assert.equal(submissions[0].plan.monthly_token_limit, 200)
    assert.equal(submissions[0].risk_confirmed, true)
    assert.equal(
      submissions[0].risk_reason,
      'approved after reviewing renewal cohorts'
    )
  })
})
