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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getAdminOpsConcurrencyUserStatus,
  getAdminOpsHealthReasonLabelKey,
  getAdminOpsHealthTone,
} from './health'

describe('admin ops health helpers', () => {
  test('maps health status to UI tone', () => {
    assert.equal(getAdminOpsHealthTone('healthy'), 'success')
    assert.equal(getAdminOpsHealthTone('degraded'), 'warning')
    assert.equal(getAdminOpsHealthTone('critical'), 'destructive')
    assert.equal(getAdminOpsHealthTone('unknown'), 'muted')
  })

  test('maps health reason codes to translated label keys', () => {
    assert.equal(
      getAdminOpsHealthReasonLabelKey('database_unhealthy'),
      'adminOps.health.reason.database_unhealthy'
    )
    assert.equal(
      getAdminOpsHealthReasonLabelKey('concurrency_redis_errors'),
      'adminOps.health.reason.concurrency_redis_errors'
    )
    assert.equal(
      getAdminOpsHealthReasonLabelKey('not_a_known_reason'),
      'adminOps.health.reason.unknown'
    )
  })

  test('classifies concurrency user pressure', () => {
    assert.equal(
      getAdminOpsConcurrencyUserStatus({
        active: 1,
        limit: 4,
        queued: 0,
        queue_capacity: 2,
      }),
      'normal'
    )
    assert.equal(
      getAdminOpsConcurrencyUserStatus({
        active: 4,
        limit: 4,
        queued: 0,
        queue_capacity: 2,
      }),
      'saturated'
    )
    assert.equal(
      getAdminOpsConcurrencyUserStatus({
        active: 4,
        limit: 4,
        queued: 1,
        queue_capacity: 2,
      }),
      'queued'
    )
    assert.equal(
      getAdminOpsConcurrencyUserStatus({
        active: 4,
        limit: 4,
        queued: 2,
        queue_capacity: 2,
      }),
      'queue_full_risk'
    )
  })
})
