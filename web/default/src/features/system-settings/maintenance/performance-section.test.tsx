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
import {
  buildPerformanceFormDefaults,
  collectPerformanceSettingUpdates,
  type PerformanceSettingsOptionValues,
} from './performance-settings-form'

function readSource(file: string): string {
  return readFileSync(file, 'utf8')
}

describe('system settings long form save actions', () => {
  test('performance page renders the top save action inside its form', () => {
    const source = readSource(
      'src/features/system-settings/maintenance/performance-section.tsx'
    )
    const formIndex = source.indexOf(`id='performance-settings-form'`)
    const topActionIndex = source.indexOf('<SettingsFormActionBar>', formIndex)
    const firstFieldIndex = source.indexOf('<FormField', formIndex)

    assert.notEqual(
      formIndex,
      -1,
      'performance page should assign performance-settings-form to the form'
    )
    assert.notEqual(
      topActionIndex,
      -1,
      'performance page should render a top save action inside the form'
    )
    assert.ok(
      topActionIndex < firstFieldIndex,
      'performance page should render the top save action before the editable fields'
    )
    assert.equal(
      source.includes(`form='performance-settings-form'`),
      false,
      'performance page should not rely on an external submitter for its top save action'
    )
  })
})

describe('performance settings form updates', () => {
  test('uses nested form values and emits flat option updates', () => {
    const defaultValues: PerformanceSettingsOptionValues = {
      'performance_setting.disk_cache_enabled': false,
      'performance_setting.disk_cache_threshold_mb': 10,
      'performance_setting.disk_cache_max_size_mb': 1024,
      'performance_setting.disk_cache_path': '',
      'performance_setting.monitor_enabled': false,
      'performance_setting.monitor_cpu_threshold': 90,
      'performance_setting.monitor_memory_threshold': 90,
      'performance_setting.monitor_disk_threshold': 95,
      'perf_metrics_setting.enabled': true,
      'perf_metrics_setting.flush_interval': 5,
      'perf_metrics_setting.bucket_time': 'hour',
      'perf_metrics_setting.retention_days': 0,
    }

    const values = buildPerformanceFormDefaults(defaultValues)

    values.performance_setting.disk_cache_enabled = true
    values.performance_setting.monitor_enabled = true
    values.perf_metrics_setting.enabled = false

    assert.deepEqual(collectPerformanceSettingUpdates(values, defaultValues), [
      {
        key: 'performance_setting.disk_cache_enabled',
        value: true,
      },
      {
        key: 'performance_setting.monitor_enabled',
        value: true,
      },
      {
        key: 'perf_metrics_setting.enabled',
        value: false,
      },
    ])
  })
})
