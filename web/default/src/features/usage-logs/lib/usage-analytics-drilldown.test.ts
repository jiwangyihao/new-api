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
import test from 'node:test'
import assert from 'node:assert/strict'

import { buildSearchParams } from './filter'
import { buildApiParams } from './utils'

test('preserves usage analytics drilldown through filter apply and api params', () => {
  const routeSearch = {
    startTime: 10000,
    endTime: 20000,
    tokenId: 5,
    model: 'gpt-4',
    group: 'default',
    isStream: true,
    status: 'success',
  }

  const nextSearch = buildSearchParams(
    {
      startTime: new Date(10000),
      endTime: new Date(20000),
      tokenId: 5,
      model: 'gpt-4',
      group: 'default',
      isStream: true,
      status: 'success',
    },
    'common'
  )
  assert.deepEqual(nextSearch, routeSearch)

  const apiParams = buildApiParams({
    page: 1,
    pageSize: 20,
    searchParams: routeSearch,
    isAdmin: false,
  })
  assert.equal(apiParams.token_id, 5)
  assert.equal(apiParams.model_name, 'gpt-4')
  assert.equal(apiParams.group, 'default')
  assert.equal(apiParams.is_stream, true)
  assert.equal(apiParams.status, 'success')
  assert.equal(Object.prototype.hasOwnProperty.call(apiParams, 'type'), false)
})

test('targets usage logs common route search without numeric type', () => {
  const search = buildSearchParams(
    {
      startTime: new Date(10000),
      endTime: new Date(20000),
      status: 'error',
    },
    'common'
  )

  assert.deepEqual(search, { startTime: 10000, endTime: 20000, status: 'error' })
})

test('does not send invalid usage analytics token id', () => {
  const apiParams = buildApiParams({
    page: 1,
    pageSize: 20,
    searchParams: { tokenId: 0, status: 'success' },
    isAdmin: false,
  })

  assert.equal(Object.prototype.hasOwnProperty.call(apiParams, 'token_id'), false)
  assert.equal(apiParams.status, 'success')
})
