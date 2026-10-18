import { describe, expect, it } from 'vitest'
import { metricRows, type RunSummary } from './compare'

const s = (over: Partial<RunSummary> = {}): RunSummary => ({ resolved: 1000, ok: 900, p50: 100, p95: 300, p99: 500, lostWrites: 0, costPerHour: 1, ...over })

describe('metricRows', () => {
  it('compares success in percentage points, higher is better', () => {
    const r = metricRows(s({ ok: 900 }), s({ ok: 990 })).find((x) => x.label === 'Success')!
    expect(r).toMatchObject({ a: '90.0%', b: '99.0%', delta: '+9.0 pts', verdict: 'B is better' })
    expect(metricRows(s({ ok: 990 }), s({ ok: 900 })).find((x) => x.label === 'Success')!.verdict).toBe('A is better')
  })
  it('compares latency and cost, lower is better', () => {
    const rows = metricRows(s({ p95: 300, costPerHour: 1 }), s({ p95: 250, costPerHour: 1.5 }))
    expect(rows.find((x) => x.label === 'p95')).toMatchObject({ a: '300 ms', b: '250 ms', delta: '-50 ms', verdict: 'B is better' })
    expect(rows.find((x) => x.label === 'Cost per hour')).toMatchObject({ a: '$1.00', b: '$1.50', delta: '+$0.50', verdict: 'A is better' })
  })
  it('calls equal values the same', () => {
    expect(metricRows(s(), s()).every((x) => x.verdict === 'Same')).toBe(true)
  })
  it('shows a dash and no verdict for latency when nothing succeeded', () => {
    const r = metricRows(s({ ok: 0 }), s()).find((x) => x.label === 'p50')!
    expect(r).toMatchObject({ a: '—', verdict: '' })
  })
  it('only lists lost writes if either side lost any', () => {
    expect(metricRows(s(), s()).some((x) => x.label === 'Writes lost')).toBe(false)
    expect(metricRows(s({ lostWrites: 12 }), s()).find((x) => x.label === 'Writes lost')).toMatchObject({ a: '12', b: '0', verdict: 'B is better' })
  })
})
