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
  const amount = Number(value.amount)
  if (!Number.isFinite(amount)) return '—'
  const currency = value.currency.trim().toUpperCase()
  if (currency === '') return amount.toFixed(2)
  const symbol = currency === 'CNY' ? '¥' : currency === 'USD' ? '$' : `${currency} `
  return `${symbol}${amount.toFixed(2)}`
}

export function formatAdminMoneyBreakdown(
  values: readonly MoneyBreakdown[] | null | undefined
): string {
  if (values === null || values === undefined || values.length === 0) return '—'
  return values.map((value) => formatAdminMoneyAmount(value)).join(', ')
}
