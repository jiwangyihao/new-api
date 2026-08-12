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
import type { MoneyAmount, MoneyBreakdown } from '../types'

export function formatAdminTokens(value: number): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(
    value
  )
}

export function formatAdminPercent(value: number | null | undefined): string {
  if (value === null || value === undefined) return '—'
  return new Intl.NumberFormat(undefined, {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(value)
}

export function formatAdminLatency(ms: number): string {
  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(ms)} ms`
}

export function formatAdminMoneyAmount(
  value: MoneyAmount | null | undefined
): string {
  if (value === null || value === undefined) return '—'
  const currency = value.currency.trim().toUpperCase()
  let decimal: string
  if (value.amount_micros !== undefined) {
    if (!/^-?\d+$/.test(value.amount_micros)) return '—'
    const micros = BigInt(value.amount_micros)
    const negative = micros < 0n
    const absoluteMicros = negative ? -micros : micros
    const roundedCents = (absoluteMicros + 5_000n) / 10_000n
    const whole = roundedCents / 100n
    const fraction = (roundedCents % 100n).toString().padStart(2, '0')
    decimal = `${negative ? '-' : ''}${whole}.${fraction}`
  } else {
    const amount = Number(value.amount)
    if (!Number.isFinite(amount)) return '—'
    decimal = amount.toFixed(2)
  }
  if (currency === '') return decimal
  const symbol = currency === 'CNY' ? '¥' : currency === 'USD' ? '$' : `${currency} `
  return `${symbol}${decimal}`
}

export function formatAdminMoneyBreakdown(
  values: readonly MoneyBreakdown[] | null | undefined
): string {
  if (values === null || values === undefined || values.length === 0) return '—'
  return values.map((value) => formatAdminMoneyAmount(value)).join(', ')
}
