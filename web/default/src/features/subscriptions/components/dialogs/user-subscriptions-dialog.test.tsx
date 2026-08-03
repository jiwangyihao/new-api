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
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, describe, test } from 'node:test'
import { I18nextProvider } from 'react-i18next'
import { api } from '@/lib/api'
import { UserSubscriptionsDialog } from './user-subscriptions-dialog'

const { cleanup, render, waitFor } = await import('@testing-library/react/pure')
const originalAPIAdapter = api.defaults.adapter

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAPIAdapter
})

describe('converted subscription admin audit', () => {
  test('shows the immutable source-to-target migration instead of an expired record', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })

    api.defaults.adapter = async (config) => {
      const data = config.url?.endsWith('/subscriptions')
        ? [
            {
              subscription: {
                id: 8201,
                user_id: 8101,
                plan_id: 8301,
                status: 'converted',
                source: 'order',
                start_time: 1_700_000_000,
                end_time: 1_700_100_000,
                amount_total: 100,
                amount_used: 25,
                converted_at: 1_789_000_000,
                conversion_id: 8401,
                converted_to_subscription_id: 8501,
              },
              conversion_audit: {
                conversion_id: '9007199254740993',
                source_subscription_id: '9007199254740995',
                target_subscription_id: '9007199254740997',
                source_status_before: 'expired',
                source_status_after: 'converted',
                target_status: 'active',
                converted_at: '1789000000',
              },
            },
          ]
        : []
      return {
        data: { success: true, data },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    const view = render(
      <I18nextProvider i18n={i18n}>
        <UserSubscriptionsDialog
          open
          onOpenChange={() => undefined}
          user={{ id: 8101, username: 'audit-user' }}
        />
      </I18nextProvider>
    )

    await waitFor(() => {
      assert.ok(view.getByText('Converted'))
    })

    const text = view.baseElement.textContent || ''
    assert.match(text, /Conversion ID:\s*#9007199254740993/)
    assert.match(
      text,
      /Source subscription:\s*#9007199254740995\s*→\s*Target Credit balance:\s*#9007199254740997/
    )
    assert.match(text, /Status:\s*expired\s*→\s*converted/)
    assert.match(text, /Target Credit balance:\s*active/)
    assert.match(text, /Converted at:/)
    assert.equal(
      (view.getByRole('button', { name: 'Invalidate' }) as HTMLButtonElement)
        .disabled,
      true
    )
    assert.equal(
      (view.getByRole('button', { name: 'Delete' }) as HTMLButtonElement)
        .disabled,
      true
    )
    assert.equal(view.queryByText('Expired'), null)
  })
})

describe('timed subscription after-sales grant', () => {
  test('submits frozen valuation facts and reuses the retry key until grant details change', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const user = userEvent.setup()
    const grants: Array<Record<string, unknown>> = []

    api.defaults.adapter = async (config) => {
      if (config.method === 'get' && config.url?.endsWith('/plans')) {
        return {
          data: {
            success: true,
            data: [
              {
                plan: {
                  id: 8301,
                  title: 'Timed Pro',
                  price_amount: 25,
                  price_amount_micros: '25000000',
                  currency: 'USD',
                  entitlement_type: 'timed',
                  enabled: true,
                  is_trial: false,
                  invite_trial: false,
                },
              },
            ],
          },
          status: 200,
          statusText: 'OK',
          headers: {},
          config,
        }
      }
      if (config.method === 'get' && config.url?.endsWith('/subscriptions')) {
        return {
          data: { success: true, data: [] },
          status: 200,
          statusText: 'OK',
          headers: {},
          config,
        }
      }
      if (config.method === 'post' && config.url?.endsWith('/subscriptions')) {
        grants.push(JSON.parse(String(config.data)) as Record<string, unknown>)
        return {
          data: { success: false, message: 'temporary upstream failure' },
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
        <UserSubscriptionsDialog
          open
          onOpenChange={() => undefined}
          user={{ id: 8101, username: 'grant-user' }}
        />
      </I18nextProvider>
    )

    const plan = await view.findByRole('combobox', {
      name: 'Timed subscription plan',
    })
    plan.focus()
    await user.keyboard('{Enter}{End}{Enter}')
    const reason = view.getByLabelText('Grant reason')
    await user.type(reason, 'approved service correction')
    const submit = view.getByRole('button', {
      name: 'Grant timed subscription',
    })

    await user.click(submit)
    await waitFor(() => assert.equal(grants.length, 1))
    assert.deepEqual(
      {
        plan_id: grants[0].plan_id,
        reason: grants[0].reason,
        source_price_micros: grants[0].source_price_micros,
        source_currency: grants[0].source_currency,
      },
      {
        plan_id: 8301,
        reason: 'approved service correction',
        source_price_micros: '25000000',
        source_currency: 'USD',
      }
    )
    assert.match(String(grants[0].idempotency_key), /^admin-timed-8101-/)
    assert.ok(view.getByText('Timed grant failed'))

    await user.click(submit)
    await waitFor(() => assert.equal(grants.length, 2))
    assert.equal(grants[1].idempotency_key, grants[0].idempotency_key)

    await user.clear(reason)
    await user.type(reason, 'different approved correction')
    await user.click(submit)
    await waitFor(() => assert.equal(grants.length, 3))
    assert.notEqual(grants[2].idempotency_key, grants[1].idempotency_key)
  })
})
