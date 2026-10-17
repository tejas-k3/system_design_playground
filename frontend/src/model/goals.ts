import type { Goals, Stats } from '../api/types'

export interface GoalRow {
  label: string
  target: string
  actual: string
  state: 'pass' | 'fail' | 'pending'
}

const pct = (f: number) => `${(Math.floor(f * 1000) / 10).toFixed(1)}%`
const ms = (v: number) => (v >= 10000 ? `${(v / 1000).toFixed(1)} s` : `${Math.round(v)} ms`)

/** Live read-out of how a run is tracking against the goals (the server gives the final verdict). */
export function goalRows(g: Goals, stats: Stats, cost: number, started: boolean): GoalRow[] {
  const r = stats.global.run
  const total = r.resolved + r.pending
  const rows: GoalRow[] = []
  const rate = total > 0 ? r.ok / total : 0
  rows.push({ label: 'Success rate', target: `≥ ${pct(g.minSuccessRate)}`, actual: total ? pct(rate) : '—', state: !started || !total ? 'pending' : rate >= g.minSuccessRate ? 'pass' : 'fail' })
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
  if (g.maxCostPerHour !== undefined) rows.push({ label: 'Cost', target: `≤ $${g.maxCostPerHour.toFixed(2)}/hr`, actual: `$${cost.toFixed(2)}/hr`, state: cost <= g.maxCostPerHour + 1e-9 ? 'pass' : 'fail' })
  return rows
}
