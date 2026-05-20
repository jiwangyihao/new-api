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
const integerFormatter = new Intl.NumberFormat('en-US', {
  maximumFractionDigits: 0,
})
const secondsFormatter = new Intl.NumberFormat('en-US', {
  maximumFractionDigits: 1,
})
const percentFormatter = new Intl.NumberFormat('en-US', {
  style: 'percent',
  maximumFractionDigits: 1,
})
const compactNumberFormatter = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 2,
})

function finiteOrZero(value: number): number {
  return Number.isFinite(value) ? value : 0
}

export function formatLatencyMs(value: number): string {
  const normalized = Math.max(0, finiteOrZero(value))
  if (normalized < 1_000) return `${integerFormatter.format(normalized)} ms`
  return `${secondsFormatter.format(normalized / 1_000)} s`
}

export function formatUsagePercent(value: number): string {
  return percentFormatter.format(Math.max(0, finiteOrZero(value)))
}

export function formatUsageTokens(value: number): string {
  return compactNumberFormatter.format(Math.max(0, finiteOrZero(value)))
}
