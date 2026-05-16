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
import * as z from 'zod'
import type { OperationsSettings, UpdateOptionRequest } from '../types'

const numberInput = (schema: z.ZodNumber) =>
  z.preprocess((value) => Number(value), schema)

export const performanceSettingsFormSchema = z.object({
  performance_setting: z.object({
    disk_cache_enabled: z.boolean(),
    disk_cache_threshold_mb: numberInput(z.number().min(1)),
    disk_cache_max_size_mb: numberInput(z.number().min(100)),
    disk_cache_path: z.string(),
    monitor_enabled: z.boolean(),
    monitor_cpu_threshold: numberInput(z.number().min(0).max(100)),
    monitor_memory_threshold: numberInput(z.number().min(0).max(100)),
    monitor_disk_threshold: numberInput(z.number().min(0).max(100)),
  }),
  perf_metrics_setting: z.object({
    enabled: z.boolean(),
    flush_interval: numberInput(z.number().min(1)),
    bucket_time: z.enum(['minute', '5min', 'hour']),
    retention_days: numberInput(z.number().min(0)),
  }),
})

export type PerformanceSettingsFormValues = z.infer<
  typeof performanceSettingsFormSchema
>

type PerformanceSettingsOptionKey =
  | 'performance_setting.disk_cache_enabled'
  | 'performance_setting.disk_cache_threshold_mb'
  | 'performance_setting.disk_cache_max_size_mb'
  | 'performance_setting.disk_cache_path'
  | 'performance_setting.monitor_enabled'
  | 'performance_setting.monitor_cpu_threshold'
  | 'performance_setting.monitor_memory_threshold'
  | 'performance_setting.monitor_disk_threshold'
  | 'perf_metrics_setting.enabled'
  | 'perf_metrics_setting.flush_interval'
  | 'perf_metrics_setting.bucket_time'
  | 'perf_metrics_setting.retention_days'

export type PerformanceSettingsOptionValues = Pick<
  OperationsSettings,
  PerformanceSettingsOptionKey
>

type PerformanceSettingsField = {
  key: PerformanceSettingsOptionKey
  getValue: (
    values: PerformanceSettingsFormValues
  ) => UpdateOptionRequest['value']
}

const PERFORMANCE_SETTINGS_FIELDS = [
  {
    key: 'performance_setting.disk_cache_enabled',
    getValue: (values) => values.performance_setting.disk_cache_enabled,
  },
  {
    key: 'performance_setting.disk_cache_threshold_mb',
    getValue: (values) => values.performance_setting.disk_cache_threshold_mb,
  },
  {
    key: 'performance_setting.disk_cache_max_size_mb',
    getValue: (values) => values.performance_setting.disk_cache_max_size_mb,
  },
  {
    key: 'performance_setting.disk_cache_path',
    getValue: (values) => values.performance_setting.disk_cache_path,
  },
  {
    key: 'performance_setting.monitor_enabled',
    getValue: (values) => values.performance_setting.monitor_enabled,
  },
  {
    key: 'performance_setting.monitor_cpu_threshold',
    getValue: (values) => values.performance_setting.monitor_cpu_threshold,
  },
  {
    key: 'performance_setting.monitor_memory_threshold',
    getValue: (values) => values.performance_setting.monitor_memory_threshold,
  },
  {
    key: 'performance_setting.monitor_disk_threshold',
    getValue: (values) => values.performance_setting.monitor_disk_threshold,
  },
  {
    key: 'perf_metrics_setting.enabled',
    getValue: (values) => values.perf_metrics_setting.enabled,
  },
  {
    key: 'perf_metrics_setting.flush_interval',
    getValue: (values) => values.perf_metrics_setting.flush_interval,
  },
  {
    key: 'perf_metrics_setting.bucket_time',
    getValue: (values) => values.perf_metrics_setting.bucket_time,
  },
  {
    key: 'perf_metrics_setting.retention_days',
    getValue: (values) => values.perf_metrics_setting.retention_days,
  },
] satisfies readonly PerformanceSettingsField[]

export function buildPerformanceFormDefaults(
  defaultValues: PerformanceSettingsOptionValues
): PerformanceSettingsFormValues {
  return {
    performance_setting: {
      disk_cache_enabled:
        defaultValues['performance_setting.disk_cache_enabled'] ?? false,
      disk_cache_threshold_mb:
        defaultValues['performance_setting.disk_cache_threshold_mb'] ?? 10,
      disk_cache_max_size_mb:
        defaultValues['performance_setting.disk_cache_max_size_mb'] ?? 1024,
      disk_cache_path:
        defaultValues['performance_setting.disk_cache_path'] ?? '',
      monitor_enabled:
        defaultValues['performance_setting.monitor_enabled'] ?? false,
      monitor_cpu_threshold:
        defaultValues['performance_setting.monitor_cpu_threshold'] ?? 90,
      monitor_memory_threshold:
        defaultValues['performance_setting.monitor_memory_threshold'] ?? 90,
      monitor_disk_threshold:
        defaultValues['performance_setting.monitor_disk_threshold'] ?? 95,
    },
    perf_metrics_setting: {
      enabled: defaultValues['perf_metrics_setting.enabled'] ?? true,
      flush_interval: defaultValues['perf_metrics_setting.flush_interval'] ?? 5,
      bucket_time: defaultValues['perf_metrics_setting.bucket_time'] ?? 'hour',
      retention_days: defaultValues['perf_metrics_setting.retention_days'] ?? 0,
    },
  }
}

export function collectPerformanceSettingUpdates(
  values: PerformanceSettingsFormValues,
  defaultValues: PerformanceSettingsOptionValues
): UpdateOptionRequest[] {
  const updates: UpdateOptionRequest[] = []

  for (const field of PERFORMANCE_SETTINGS_FIELDS) {
    const value = field.getValue(values)

    if (value !== defaultValues[field.key]) {
      updates.push({ key: field.key, value })
    }
  }

  return updates
}
