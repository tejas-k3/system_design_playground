import type { Diagram, Finding } from '../api/types'
import type { InterviewData, Requirement, RubricArea } from './interview'

export interface TargetRow {
  label: string
  target: string
  actual: string
  ok: boolean | null // null: cannot be tested
}

/** Non-functional requirements checked against what a run of the design measured. Only availability/success (%) and latency (ms) can be tested. */
export function checkTargets(reqs: Requirement[], run: { success: number; p95: number }): TargetRow[] {
  return reqs.map((r) => {
    if (r.target === null) return { label: r.label, target: '—', actual: 'no number yet', ok: null }
    const target = `${r.target}${r.unit ? ' ' + r.unit : ''}`
    const name = r.label.toLowerCase()
    if (r.unit === '%' && /availab|success|uptime/.test(name)) {
      const actual = run.success * 100
      return { label: r.label, target, actual: `${Math.round(actual * 100) / 100} %`, ok: actual >= r.target }
    }
    if (r.unit === 'ms' && /latenc|p9|response|time/.test(name)) return { label: r.label, target, actual: `${Math.round(run.p95)} ms`, ok: run.p95 <= r.target }
    return { label: r.label, target, actual: 'cannot be tested by a run', ok: null }
  })
}

export interface Fact {
  area: RubricArea
  text: string
  ok: boolean | null // null: neither good nor bad, just a measurement
}

interface Inputs {
  data: InterviewData
  findings: Finding[]
  run: { success: number; p95: number; costPerHour: number } | null
}

/** Facts from the design, the lint and the last run that a self-rating can lean on. */
export function rubricEvidence(diagram: Diagram, x: Inputs): Fact[] {
  const out: Fact[] = []
  const has = (rule: string) => x.findings.some((f) => f.rule === rule)
  const apps = diagram.nodes.filter((n) => n.kind === 'appServer')
  const auto = apps.some((n) => n.params?.autoscale)
  const pct = (v: number) => `${Math.round(v * 1000) / 10} %`

  out.push(
    apps.length > 1 || auto
      ? { area: 'scalability', text: auto ? 'App servers autoscale with load.' : `${apps.length} app servers share the load.`, ok: true }
      : { area: 'scalability', text: 'Only one app server: it cannot grow past its own limits.', ok: false },
  )
  if (x.run) out.push({ area: 'scalability', text: `Last run: ${pct(x.run.success)} succeeded, p95 ${Math.round(x.run.p95)} ms.`, ok: null })

  out.push({ area: 'availability', text: has('spof-database') ? 'The only copy of the data is on one database.' : 'The data is not on a lone database.', ok: !has('spof-database') })
  out.push({ area: 'availability', text: has('no-call-timeout') ? 'App servers wait forever on a silent dependency (no call timeout).' : 'Calls to dependencies have timeouts.', ok: !has('no-call-timeout') })
  out.push({ area: 'availability', text: has('spof-server') ? 'A single app server is a single point of failure.' : 'No single app server is a single point of failure.', ok: !has('spof-server') })

  out.push({ area: 'consistency', text: has('unfenced-failover') ? 'Failover without fencing allows split brain.' : 'No unfenced failover (no split-brain risk from a promotion).', ok: !has('unfenced-failover') })
  out.push({ area: 'consistency', text: has('cache-no-ttl-with-writes') ? 'A cache that never expires or updates serves stale data to readers who need fresh data.' : 'Caches are kept honest when data is written.', ok: !has('cache-no-ttl-with-writes') })

  out.push(x.run ? { area: 'cost', text: `Last run: $${x.run.costPerHour.toFixed(2)} an hour.`, ok: null } : { area: 'cost', text: 'Run the design to see what it costs an hour.', ok: null })
  out.push({ area: 'cost', text: has('autoscale-max-low') ? 'The autoscaler ceiling is below what the traffic needs.' : 'No design finding about wasted or missing capacity.', ok: !has('autoscale-max-low') })

  const d = x.data
  out.push({ area: 'communication', text: `${d.functional.filter((s) => s.trim()).length} functional requirements written (aim for three).`, ok: d.functional.filter((s) => s.trim()).length >= 3 })
  const withNumbers = d.nonFunctional.filter((r) => r.target !== null).length
  out.push({ area: 'communication', text: `${withNumbers} non-functional requirements with numbers (aim for three).`, ok: withNumbers >= 3 })
  out.push({ area: 'communication', text: d.notes.trim() ? 'You wrote notes about your choices.' : 'No notes about your choices or what you gave up.', ok: d.notes.trim().length > 0 })
  return out
}
