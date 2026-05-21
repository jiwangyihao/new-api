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
