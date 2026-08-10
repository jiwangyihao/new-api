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
import { useState } from 'react'
import { cleanup, fireEvent, render } from '@testing-library/react/pure'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'
import { I18nextProvider } from 'react-i18next'
import { RedemptionCodeCard } from '../redemption-code-card'

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

test('submits the entered redemption code from its standalone card', async () => {
  const i18n = await createTestI18n()
  const submitted: string[] = []

  function TestCard() {
    const [code, setCode] = useState('')
    return (
      <RedemptionCodeCard
        code={code}
        redeeming={false}
        topupLink='https://example.com/codes'
        onCodeChange={setCode}
        onRedeem={() => {
          submitted.push(code)
        }}
      />
    )
  }

  const view = render(
    <I18nextProvider i18n={i18n}>
      <TestCard />
    </I18nextProvider>
  )
  const input = view.getByRole('textbox', { name: 'Redemption Code' })

  fireEvent.change(input, { target: { value: 'subscription-code' } })
  fireEvent.click(view.getByRole('button', { name: 'Redeem' }))

  assert.deepEqual(submitted, ['subscription-code'])
  assert.equal(
    view.getByRole('link', { name: 'Get one here' }).getAttribute('href'),
    'https://example.com/codes'
  )
})
