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
export function formatAdminOpsCount(value: number): string {
  if (!Number.isFinite(value)) return '0'
  const abs = Math.abs(value)
  if (abs >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (abs >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return Math.round(value).toString()
}

export function formatAdminOpsPercent(value: number): string {
  if (!Number.isFinite(value)) return '0.0%'
  return `${(value * 100).toFixed(1)}%`
}

export function formatAdminOpsDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s'
  const whole = Math.floor(seconds)
  if (whole < 60) return `${whole}s`
  const minutes = Math.floor(whole / 60)
  const restSeconds = whole % 60
  if (minutes < 60) return `${minutes}m ${restSeconds}s`
  const hours = Math.floor(minutes / 60)
  const restMinutes = minutes % 60
  return `${hours}h ${restMinutes}m`
}

export function formatAdminOpsRate(value: number, unit: string): string {
  if (!Number.isFinite(value)) return `0.0 ${unit}`
  return `${value.toFixed(1)} ${unit}`
}

export function formatAdminOpsBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB'] as const
  let current = value
  let unitIndex = 0
  for (; unitIndex < units.length - 1 && current >= 1024; unitIndex += 1) {
    current /= 1024
  }
  if (unitIndex === 0) return `${Math.round(current)} ${units[unitIndex]}`
  return `${current.toFixed(1)} ${units[unitIndex]}`
}

export function formatAdminOpsUsage(used: number, total: number): string {
  if (!Number.isFinite(used) || used < 0) used = 0
  if (!Number.isFinite(total) || total <= 0)
    return `${formatAdminOpsCount(used)}/∞`
  return `${formatAdminOpsCount(used)}/${formatAdminOpsCount(total)}`
}
