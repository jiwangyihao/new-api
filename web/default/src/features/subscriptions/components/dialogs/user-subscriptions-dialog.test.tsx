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
