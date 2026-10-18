import type { SeriesPoint } from '../api/types'

const round = (n: number) => (Math.abs(n) >= 100 ? Math.round(n) : Math.round(n * 10) / 10)

/** A sentence for screen readers: where a chart starts, its peak and low with times, and where it ends. */
export function describeSeries(name: string, series: SeriesPoint[], pick: (p: SeriesPoint) => number, unit: string): string {
  if (series.length === 0) return `${name}: no data.`
  let hi = series[0]
  let lo = series[0]
  for (const p of series) {
    if (pick(p) > pick(hi)) hi = p
    if (pick(p) < pick(lo)) lo = p
  }
  if (pick(hi) === pick(lo)) return `${name}, ${unit}. Flat at ${round(pick(hi))}.`
  const first = series[0]
  const last = series[series.length - 1]
  return `${name}, ${unit}. Starts at ${round(pick(first))}, peaks at ${round(pick(hi))} (${Math.round(hi.t)} s), lowest ${round(pick(lo))} (${Math.round(lo.t)} s), ends at ${round(pick(last))}.`
}
