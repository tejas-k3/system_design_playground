import type { Goals, Stats } from '../api/types'

export interface GoalRow {
  label: string
  target: string
  actual: string
  state: 'pass' | 'fail' | 'pending'
}

/** A percentage rounded down to a tenth, to a hundredth below a tenth of a percent, and shown as under 0.01% below that, so a small limit never reads as zero. */
export const pct = (f: number) => {
  if (f > 0 && f < 0.0001) return '<0.01%'
  if (f > 0 && f < 0.001) return `${(Math.floor(f * 10000 + 1e-9) / 100).toFixed(2)}%`
  return `${(Math.floor(f * 1000 + 1e-9) / 10).toFixed(1)}%`
}
const ms = (v: number) => (v >= 10000 ? `${(v / 1000).toFixed(1)} s` : `${Math.round(v)} ms`)

/** Hourly cost as shown on screen; a dash until the server has priced the design. */
export const fmtCost = (c: number | null): string => (c === null ? '—' : `$${c.toFixed(2)}/hr`)

/** Median time a failed request took (the headline latencies count successes only). */
export const failedAfter = (r: Stats['global']['run']): string => (r.failed > 0 ? ms(r.failP50) : '—')

/** The cost a goal row should judge: the run average while running (an autoscaled fleet's live cost swings, the score uses the average). */
export function goalCost(cost: number | null, stats: Stats, started: boolean): number | null {
  return started && stats.global.avgCost > 0 ? stats.global.avgCost : cost
}

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
  if (g.minDeliveriesPerMessage !== undefined) { const ok = r.ok > 0 && r.jobs; const v = ok ? (r.jobs!.completed / r.ok) : 0; rows.push({ label: 'Deliveries per message', target: `≥ ${g.minDeliveriesPerMessage.toFixed(1)}`, actual: ok ? v.toFixed(2) : 'no messages', state: !started ? 'pending' : ok && v >= g.minDeliveriesPerMessage ? 'pass' : 'fail' }) }
  if (g.minWorstTenant !== undefined) { const t = r.tenants ?? 0; const v = r.worstTenantSuccess ?? 0; rows.push({ label: 'Worst-served quiet tenant', target: `≥ ${(g.minWorstTenant * 100).toFixed(1)}%`, actual: t > 1 ? `${(v * 100).toFixed(1)}%` : 'no tenants', state: !started ? 'pending' : t > 1 && v >= g.minWorstTenant ? 'pass' : 'fail' }) }
  if (g.maxInconsistent !== undefined) { const v = r.inconsistent ?? 0; rows.push({ label: 'Orders left half done', target: `≤ ${g.maxInconsistent}`, actual: String(v), state: !started ? 'pending' : v <= g.maxInconsistent ? 'pass' : 'fail' }) }
  if (g.maxBlockedSec !== undefined) { const v = r.blockedSec ?? 0; rows.push({ label: 'Lock-seconds held for a dead coordinator', target: `≤ ${g.maxBlockedSec.toFixed(1)} s`, actual: `${v.toFixed(1)} s`, state: !started ? 'pending' : v <= g.maxBlockedSec + 1e-9 ? 'pass' : 'fail' }) }
  if (g.maxRepeatedOutputs !== undefined) { const v = r.repeatedOutputs ?? 0; rows.push({ label: 'Outputs repeated after a crash', target: `≤ ${g.maxRepeatedOutputs}`, actual: String(v), state: !started ? 'pending' : v <= g.maxRepeatedOutputs ? 'pass' : 'fail' }) }
  if (g.maxRecoverySec !== undefined) { const v = r.recoverySec ?? 0; rows.push({ label: 'Time to catch up after a crash', target: `≤ ${g.maxRecoverySec.toFixed(1)} s`, actual: `${v.toFixed(2)} s`, state: !started ? 'pending' : v <= g.maxRecoverySec + 1e-9 ? 'pass' : 'fail' }) }
  if (g.maxWrongAnswers !== undefined) { const v = r.wrongAnswers ?? 0; rows.push({ label: 'Wrong answers accepted', target: `≤ ${g.maxWrongAnswers}`, actual: String(v), state: !started ? 'pending' : v <= g.maxWrongAnswers ? 'pass' : 'fail' }) }
  if (g.maxClockLost !== undefined) { const v = r.clockLost ?? 0; rows.push({ label: 'Newer writes lost to a clock', target: `≤ ${g.maxClockLost}`, actual: String(v), state: !started ? 'pending' : v <= g.maxClockLost ? 'pass' : 'fail' }) }
  if (g.maxReauth !== undefined) { const v = r.reauth ?? 0; rows.push({ label: 'Users signed out by a lost session', target: `≤ ${g.maxReauth}`, actual: String(v), state: !started ? 'pending' : v <= g.maxReauth ? 'pass' : 'fail' }) }
  if (g.maxUnchecked !== undefined) { const v = r.unchecked ?? 0; rows.push({ label: 'Requests let through unchecked', target: `≤ ${g.maxUnchecked}`, actual: String(v), state: !started ? 'pending' : v <= g.maxUnchecked ? 'pass' : 'fail' }) }
  if (g.maxInDoubt !== undefined) { const v = r.inDoubt ?? 0; rows.push({ label: 'Transactions waiting on a dead coordinator', target: `≤ ${g.maxInDoubt}`, actual: String(v), state: !started ? 'pending' : v <= g.maxInDoubt ? 'pass' : 'fail' }) }
  if (g.minRecall !== undefined) { const v = r.recall ?? 0; rows.push({ label: 'Vector search recall', target: `≥ ${Math.round(g.minRecall * 100)}%`, actual: v === 0 ? '—' : `${Math.round(v * 100)}%`, state: !started || v === 0 ? 'pending' : v + 1e-9 >= g.minRecall ? 'pass' : 'fail' }) }
  if (g.maxTTFTMs !== undefined) { const v = r.ttftMs ?? 0; rows.push({ label: 'Time to first token (p95)', target: `≤ ${g.maxTTFTMs} ms`, actual: v === 0 ? '—' : `${Math.round(v)} ms`, state: !started || v === 0 ? 'pending' : v <= g.maxTTFTMs ? 'pass' : 'fail' }) }
  if (g.minTopKRecall !== undefined) { const v = r.topKRecall ?? 0; rows.push({ label: 'Top ten found', target: `≥ ${Math.round(g.minTopKRecall * 10)} of 10`, actual: `${Math.round(v * 10)} of 10`, state: !started ? 'pending' : v + 1e-9 >= g.minTopKRecall ? 'pass' : 'fail' }) }
  if (g.maxStateKB !== undefined) { const v = r.stateKB ?? 0; rows.push({ label: 'Memory to count visitors', target: `≤ ${g.maxStateKB} KB`, actual: `${v.toFixed(1)} KB`, state: !started ? 'pending' : v <= g.maxStateKB ? 'pass' : 'fail' }) }
  if (g.maxCountErrorPct !== undefined) { const v = r.countErrorPct ?? 0; rows.push({ label: 'Error of the visitor count', target: `≤ ${g.maxCountErrorPct}%`, actual: `${v.toFixed(1)}%`, state: !started ? 'pending' : v <= g.maxCountErrorPct ? 'pass' : 'fail' }) }
  if (g.maxLateDropped !== undefined) { const v = r.lateDropped ?? 0; rows.push({ label: 'Late events dropped', target: `≤ ${g.maxLateDropped}`, actual: String(v), state: !started ? 'pending' : v <= g.maxLateDropped ? 'pass' : 'fail' }) }
  if (g.maxResultDelaySec !== undefined) { const v = r.resultDelaySec ?? 0; rows.push({ label: "Wait for a window's first result", target: `≤ ${g.maxResultDelaySec} s`, actual: `${v} s`, state: !started ? 'pending' : v <= g.maxResultDelaySec ? 'pass' : 'fail' }) }
  if (g.maxLostEvents !== undefined) { const v = r.eventsLost ?? 0; rows.push({ label: 'Events lost', target: `≤ ${g.maxLostEvents}`, actual: String(v), state: !started ? 'pending' : v <= g.maxLostEvents ? 'pass' : 'fail' }) }
  if (g.maxRpoSec !== undefined) { const v = r.rpoSec ?? 0; rows.push({ label: 'Data lost (RPO)', target: `≤ ${g.maxRpoSec} s`, actual: `${v.toFixed(1)} s`, state: !started ? 'pending' : v <= g.maxRpoSec ? 'pass' : 'fail' }) }
  if (g.maxRtoSec !== undefined) { const v = r.rtoSec ?? 0; rows.push({ label: 'Time to restore (RTO)', target: `≤ ${g.maxRtoSec} s`, actual: `${v.toFixed(1)} s`, state: !started ? 'pending' : v <= g.maxRtoSec ? 'pass' : 'fail' }) }
  if (g.maxSessionViolations !== undefined) { const broken = (r.readYourWritesBroken ?? 0) + (r.monotonicBroken ?? 0) + (r.monotonicWritesBroken ?? 0) + (r.writesFollowReadsBroken ?? 0) + (r.causalBroken ?? 0); rows.push({ label: 'Per-client guarantees broken', target: `≤ ${g.maxSessionViolations}`, actual: String(broken), state: !started ? 'pending' : broken <= g.maxSessionViolations ? 'pass' : 'fail' }) }
  if (g.maxCacheMessages !== undefined) { const sent = r.cacheMessages ?? 0; rows.push({ label: 'Messages to caches', target: `≤ ${g.maxCacheMessages.toLocaleString('en-US')}`, actual: sent.toLocaleString('en-US'), state: !started ? 'pending' : sent <= g.maxCacheMessages ? 'pass' : 'fail' }) }
  if (g.maxLostUpdates !== undefined) { const lost = r.lostUpdates ?? 0; rows.push({ label: 'Lost updates', target: `≤ ${g.maxLostUpdates}`, actual: String(lost), state: !started ? 'pending' : lost <= g.maxLostUpdates ? 'pass' : 'fail' }) }
  if (g.minCustomerSuccess !== undefined) { const t = r.customerTotal ?? 0; const f = t ? (r.customerOk ?? 0) / t : 0; rows.push({ label: 'Customers served', target: `≥ ${pct(g.minCustomerSuccess)}`, actual: t ? pct(f) : '—', state: !started || !t ? 'pending' : f >= g.minCustomerSuccess ? 'pass' : 'fail' }) }
  if (g.maxJobsLost !== undefined) { const lost = r.jobs?.lost ?? 0; rows.push({ label: 'Jobs lost', target: `≤ ${g.maxJobsLost}`, actual: String(lost), state: !started ? 'pending' : lost <= g.maxJobsLost ? 'pass' : 'fail' }) }
  if (g.maxWriteSkew !== undefined) { const sk = r.writeSkew ?? 0; rows.push({ label: 'Rule broken (write skew)', target: `≤ ${g.maxWriteSkew}`, actual: String(sk), state: !started ? 'pending' : sk <= g.maxWriteSkew ? 'pass' : 'fail' }) }
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
