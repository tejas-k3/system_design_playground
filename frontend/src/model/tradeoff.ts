import type { RunSummary } from '../api/types'

export interface Column {
  label: string
  format: (r: RunSummary) => string
}

const ms = (v: number) => (v >= 1000 ? `${(v / 1000).toFixed(1)} s` : `${Math.round(v)} ms`)
const pct = (v: number) => `${(v * 100).toFixed(1)}%`

/** The columns worth showing for a set of runs: the always-relevant ones, plus lost writes and stale reads only if any run had them. */
export function comparisonColumns(runs: RunSummary[]): Column[] {
  const cols: Column[] = [
    { label: 'Success', format: (r) => pct(r.success) },
    { label: 'p95', format: (r) => ms(r.p95) },
    { label: 'p99', format: (r) => ms(r.p99) },
    { label: 'Cost per hour', format: (r) => `$${r.costPerHour.toFixed(2)}` },
  ]
  if (runs.some((r) => r.lostWrites > 0)) cols.push({ label: 'Acknowledged writes lost', format: (r) => String(r.lostWrites) })
  if (runs.some((r) => r.staleReadPct > 0)) cols.push({ label: 'Stale reads', format: (r) => pct(r.staleReadPct) })
  return cols
}
