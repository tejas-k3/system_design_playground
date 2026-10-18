export interface RunSummary {
  resolved: number
  ok: number
  p50: number
  p95: number
  p99: number
  lostWrites: number
  costPerHour: number
}
export interface MetricRow {
  label: string
  a: string
  b: string
  delta: string
  verdict: 'A is better' | 'B is better' | 'Same' | ''
}

function verdict(a: number, b: number, lowerIsBetter: boolean): MetricRow['verdict'] {
  if (a === b) return 'Same'
  return (b < a) === lowerIsBetter ? 'B is better' : 'A is better'
}
const sign = (n: number) => (n > 0 ? '+' : n < 0 ? '-' : '')

/** Side-by-side metrics for two runs of the same scenario seed; the words, not just a colour, say who wins. */
export function metricRows(a: RunSummary, b: RunSummary): MetricRow[] {
  const rate = (r: RunSummary) => (r.resolved > 0 ? Math.round((r.ok / r.resolved) * 1000) / 10 : 0)
  const rows: MetricRow[] = [
    { label: 'Success', a: `${rate(a).toFixed(1)}%`, b: `${rate(b).toFixed(1)}%`, delta: `${sign(rate(b) - rate(a))}${Math.abs(rate(b) - rate(a)).toFixed(1)} pts`, verdict: verdict(rate(a), rate(b), false) },
  ]
  for (const k of ['p50', 'p95', 'p99'] as const) {
    if (a.ok === 0 || b.ok === 0) {
      rows.push({ label: k, a: a.ok === 0 ? '—' : `${Math.round(a[k])} ms`, b: b.ok === 0 ? '—' : `${Math.round(b[k])} ms`, delta: '', verdict: '' })
      continue
    }
    const x = Math.round(a[k])
    const y = Math.round(b[k])
    rows.push({ label: k, a: `${x} ms`, b: `${y} ms`, delta: `${sign(y - x)}${Math.abs(y - x)} ms`, verdict: verdict(x, y, true) })
  }
  const ca = Math.round(a.costPerHour * 100) / 100
  const cb = Math.round(b.costPerHour * 100) / 100
  rows.push({ label: 'Cost per hour', a: `$${ca.toFixed(2)}`, b: `$${cb.toFixed(2)}`, delta: `${sign(cb - ca)}$${Math.abs(cb - ca).toFixed(2)}`, verdict: verdict(ca, cb, true) })
  if (a.lostWrites > 0 || b.lostWrites > 0) {
    rows.push({ label: 'Writes lost', a: String(a.lostWrites), b: String(b.lostWrites), delta: `${sign(b.lostWrites - a.lostWrites)}${Math.abs(b.lostWrites - a.lostWrites)}`, verdict: verdict(a.lostWrites, b.lostWrites, true) })
  }
  return rows
}
