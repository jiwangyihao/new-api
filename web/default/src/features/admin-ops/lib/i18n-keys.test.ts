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
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

type LocaleFile = {
  translation?: Record<string, unknown>
}

const localeNames = ['en', 'zh', 'fr', 'ja', 'ru', 'vi'] as const

const requiredAdminOpsKeys = [
  'adminOps.title',
  'adminOps.description',
  'adminOps.failedToLoad',
  'adminOps.failedToLoadDescription',
  'adminOps.header.lastUpdated',
  'adminOps.header.autoRefresh',
  'adminOps.header.manualRefresh',
  'adminOps.header.refreshing',
  'adminOps.health.healthy',
  'adminOps.health.degraded',
  'adminOps.health.critical',
  'adminOps.health.score',
  'adminOps.health.reasons',
  'adminOps.health.reason.database_unhealthy',
  'adminOps.health.reason.redis_unhealthy',
  'adminOps.health.reason.system_cpu_high',
  'adminOps.health.reason.system_memory_high',
  'adminOps.health.reason.system_disk_high',
  'adminOps.health.reason.concurrency_queue_full_rejections',
  'adminOps.health.reason.concurrency_redis_errors',
  'adminOps.health.reason.concurrency_unavailable_rejections',
  'adminOps.health.reason.concurrency_queue_not_empty',
  'adminOps.health.reason.concurrency_saturated_users',
  'adminOps.health.reason.concurrency_queue_pressure_high',
  'adminOps.health.reason.channel_auto_disabled',
  'adminOps.health.reason.traffic_error_rate_high',
  'adminOps.health.reason.unknown',
  'adminOps.healthSummary.title',
  'adminOps.healthSummary.database',
  'adminOps.healthSummary.redis',
  'adminOps.healthSummary.cpu',
  'adminOps.healthSummary.memory',
  'adminOps.healthSummary.disk',
  'adminOps.healthSummary.activeConnections',
  'adminOps.healthSummary.goroutines',
  'adminOps.dependency.healthy',
  'adminOps.dependency.disabled',
  'adminOps.dependency.critical',
  'adminOps.concurrency.title',
  'adminOps.concurrency.activeSlots',
  'adminOps.concurrency.queuedRequests',
  'adminOps.concurrency.activeUsers',
  'adminOps.concurrency.queuedUsers',
  'adminOps.concurrency.saturatedUsers',
  'adminOps.concurrency.queuePressure',
  'adminOps.concurrency.acquiredTotal',
  'adminOps.concurrency.queuedTotal',
  'adminOps.concurrency.queueFullRejections',
  'adminOps.concurrency.unavailableRejections',
  'adminOps.concurrency.redisErrors',
  'adminOps.concurrency.mode',
  'adminOps.concurrency.ttl',
  'adminOps.concurrency.defaultQueueCapacity',
  'adminOps.concurrency.requireRedis',
  'adminOps.concurrency.failOpen',
  'adminOps.concurrency.active',
  'adminOps.concurrency.queued',
  'adminOps.concurrency.oldestQueued',
  'adminOps.concurrency.utilization',
  'adminOps.concurrency.queueUtilization',
  'adminOps.concurrency.status.normal',
  'adminOps.concurrency.status.saturated',
  'adminOps.concurrency.status.queued',
  'adminOps.concurrency.status.queue_full_risk',
  'adminOps.traffic.title',
  'adminOps.traffic.requests',
  'adminOps.traffic.errors',
  'adminOps.traffic.rpm',
  'adminOps.traffic.tpm',
  'adminOps.traffic.errorRate',
  'adminOps.traffic.window',
  'adminOps.channels.title',
  'adminOps.channels.total',
  'adminOps.channels.enabled',
  'adminOps.channels.manualDisabled',
  'adminOps.channels.autoDisabled',
  'adminOps.channels.slow',
  'adminOps.channels.staleTest',
  'adminOps.performance.title',
  'adminOps.performance.model',
  'adminOps.performance.avgLatency',
  'adminOps.performance.avgTtft',
  'adminOps.performance.successRate',
  'adminOps.performance.avgTps',
  'adminOps.performance.requestCount',
  'adminOps.recentErrors.title',
  'adminOps.recentErrors.empty',
  'adminOps.recentErrors.model',
  'adminOps.recentErrors.channel',
  'adminOps.recentErrors.requestId',
  'adminOps.recentErrors.createdAt',
  'adminOps.empty.noData',
  'adminOps.mode.redis',
  'adminOps.mode.memory',
  'adminOps.mode.disabled',
] as const

function readJson<T>(relativePath: string): T {
  return JSON.parse(
    readFileSync(new URL(relativePath, import.meta.url), 'utf8')
  )
}

describe('admin ops i18n keys', () => {
  test('exist in every supported locale with non-empty translations', () => {
    for (const localeName of localeNames) {
      const locale = readJson<LocaleFile>(
        `../../../i18n/locales/${localeName}.json`
      )
      assert.ok(
        locale.translation && typeof locale.translation === 'object',
        `${localeName}.json must contain a translation object`
      )

      for (const key of requiredAdminOpsKeys) {
        assert.equal(
          Object.prototype.hasOwnProperty.call(locale.translation, key),
          true,
          `${localeName}.json is missing ${key}`
        )
        const value: unknown = locale.translation[key]
        assert.equal(
          typeof value,
          'string',
          `${localeName}.${key} must be string`
        )
        assert.notEqual(
          (value as string).trim(),
          '',
          `${localeName}.${key} must not be empty`
        )
      }
    }
  })

  test('admin ops route is present in the generated route tree', () => {
    const routeTreeSource = readFileSync(
      new URL('../../../routeTree.gen.ts', import.meta.url),
      'utf8'
    )

    assert.match(routeTreeSource, /admin-ops/)
  })
})
