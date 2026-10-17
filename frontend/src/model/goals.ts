import type { Goals, Stats } from '../api/types'

export interface GoalRow {
  label: string
  target: string
  actual: string
  state: 'pass' | 'fail' | 'pending'
}

export const pct = (f: number) => `${(Math.floor(f * 1000) / 10).toFixed(1)}%`
const ms = (v: number) => (v >= 10000 ? `${(v / 1000).toFixed(1)} s` : `${Math.round(v)} ms`)

/** Hourly cost as shown on screen; a dash until the server has priced the design. */
export const fmtCost = (c: number | null): string => (c === null ? '—' : `$${c.toFixed(2)}/hr`)

/** Median time a failed request took (the headline latencies count successes only). */
export const failedAfter = (r: Stats['global']['run']): string => (r.failed > 0 ? ms(r.failP50) : '—')

/** Share of finished requests that succeeded (null before any finish). The one success figure shown anywhere on screen;
 *  requests still in flight count once they resolve, and the final verdict agrees because nothing is pending by then. */
export function successRate(r: Stats['global']['run']): number | null {
  return r.resolved > 0 ? r.ok / r.resolved : null
}

/** What the extra stars need, in words ('' when a challenge has no star targets). */
export function starTargets(g: Goals): string {
  const parts: string[] = []
  if (g.starCostPerHour !== undefined) parts.push(`2 need cost ≤ $${g.starCostPerHour.toFixed(2)}/hr`)
  if (g.starP95Ms !== undefined) parts.push(`${parts.length ? '3 also' : '3'} need p95 ≤ ${ms(g.starP95Ms)}`)
  return parts.length ? `Stars: ${parts.join(', ')}.` : ''
}

/** Live read-out of how a run is tracking against the goals (the server gives the final verdict). */
export function goalRows(g: Goals, stats: Stats, cost: number | null, started: boolean): GoalRow[] {
  const r = stats.global.run
  const rows: GoalRow[] = []
  const rate = successRate(r)
  rows.push({ label: 'Success rate', target: `≥ ${pct(g.minSuccessRate)}`, actual: rate === null ? '—' : pct(rate), state: !started || rate === null ? 'pending' : rate >= g.minSuccessRate ? 'pass' : 'fail' })
  const lat = (label: string, limit: number | undefined, v: number) => {
    if (limit === undefined) return
    rows.push({ label, target: `≤ ${ms(limit)}`, actual: r.ok ? ms(v) : '—', state: !started || !r.ok ? 'pending' : v <= limit ? 'pass' : 'fail' })
  }
  lat('Median latency', g.maxP50Ms, r.p50)
  lat('p95 latency', g.maxP95Ms, r.p95)
  if (g.maxConflicts !== undefined) rows.push({ label: 'Double bookings', target: `≤ ${g.maxConflicts}`, actual: String(r.conflicts), state: !started ? 'pending' : r.conflicts <= g.maxConflicts ? 'pass' : 'fail' })
  if (g.maxStaleReadPct !== undefined) {
    const f = r.freshReads ? r.staleReads / r.freshReads : 0
    rows.push({ label: 'Too-stale reads', target: `≤ ${pct(g.maxStaleReadPct)}`, actual: r.freshReads ? pct(f) : '—', state: !started || !r.freshReads ? 'pending' : f <= g.maxStaleReadPct ? 'pass' : 'fail' })
  }
  if (g.maxDuplicates !== undefined) rows.push({ label: 'Duplicate writes', target: `≤ ${g.maxDuplicates}`, actual: String(r.duplicates), state: !started ? 'pending' : r.duplicates <= g.maxDuplicates ? 'pass' : 'fail' })
  if (g.maxLostWrites !== undefined) rows.push({ label: 'Writes lost in a crash', target: `≤ ${g.maxLostWrites}`, actual: String(r.lostWrites), state: !started ? 'pending' : r.lostWrites <= g.maxLostWrites ? 'pass' : 'fail' })
  if (g.minJobsCompleted !== undefined) {
    const j = r.jobs
    const f = j && j.enqueued ? j.completed / j.enqueued : 0
    rows.push({ label: 'Jobs processed', target: `≥ ${pct(g.minJobsCompleted)}`, actual: j && j.enqueued ? pct(f) : '—', state: !started || !j?.enqueued ? 'pending' : f >= g.minJobsCompleted ? 'pass' : 'fail' })
  }
  if (g.maxJobP95Ms !== undefined) {
    const j = r.jobs
    rows.push({ label: 'Job delay p95', target: `≤ ${ms(g.maxJobP95Ms)}`, actual: j?.completed ? ms(j.p95) : '—', state: !started || !j?.completed ? 'pending' : j.p95 <= g.maxJobP95Ms ? 'pass' : 'fail' })
  }
  if (g.maxCostPerHour !== undefined) rows.push({ label: 'Cost', target: `≤ $${g.maxCostPerHour.toFixed(2)}/hr`, actual: fmtCost(cost), state: cost === null ? 'pending' : cost <= g.maxCostPerHour + 1e-9 ? 'pass' : 'fail' })
  return rows
}
