export function optionalNumericSearchParam(value: unknown): number | undefined {
  const candidate = Array.isArray(value) ? value[0] : value
  const raw =
    typeof candidate === 'number' || typeof candidate === 'string'
      ? String(candidate).trim()
      : ''
  if (!/^\d+$/.test(raw)) return undefined
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined
}
