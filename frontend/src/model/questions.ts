const SUFFIX: Record<string, number> = { k: 1e3, m: 1e6, g: 1e9, t: 1e12 }

/** A positive number from what a person types: 2,314 / 1.5k / 2M / 3e6. Null for anything else. */
export function parseNumber(text: string): number | null {
  const t = text.trim().replace(/,/g, '').toLowerCase()
  const m = /^(\d+(?:\.\d+)?(?:e[+-]?\d+)?)\s*([kmgt])?$/.exec(t)
  if (!m) return null
  const n = Number(m[1]) * (m[2] ? SUFFIX[m[2]] : 1)
  return Number.isFinite(n) && n > 0 ? n : null
}

/** Compact display of an expected value: whole numbers with separators above 100, otherwise two significant figures or so. */
export function formatEstimate(n: number): string {
  if (n >= 100) return Math.round(n).toLocaleString('en-US')
  if (n >= 10) return n.toFixed(1).replace(/\.0$/, '')
  if (n >= 1) return Number.isInteger(n) ? String(n) : n.toFixed(1).replace(/\.0$/, '')
  return n.toFixed(2)
}

export function verdictLabel(v: 'good' | 'close' | 'off'): string {
  return v === 'good' ? 'Good: within 30%' : v === 'close' ? 'Close: within 2×' : 'Off by more than 2×'
}
