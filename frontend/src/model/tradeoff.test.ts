import { describe, expect, it } from 'vitest'
import type { RunSummary } from '../api/types'
import { comparisonColumns } from './tradeoff'

const r = (over: Partial<RunSummary> = {}): RunSummary => ({ resolved: 1000, success: 0.87, p50: 124, p95: 167, p99: 192, costPerHour: 0.87, lostWrites: 0, staleReadPct: 0, ...over })

describe('comparisonColumns', () => {
  it('always shows success, p95 and cost with units', () => {
    const cols = comparisonColumns([r()])
    expect(cols.map((c) => c.label)).toEqual(['Success', 'p95', 'p99', 'Cost per hour'])
    expect(cols.map((c) => c.format(r()))).toEqual(['87.0%', '167 ms', '192 ms', '$0.87'])
  })
  it('adds lost writes only when some design lost any', () => {
    const cols = comparisonColumns([r(), r({ lostWrites: 248 })])
    const lost = cols.find((c) => c.label === 'Acknowledged writes lost')
    expect(lost?.format(r({ lostWrites: 248 }))).toBe('248')
    expect(comparisonColumns([r()]).some((c) => c.label === 'Acknowledged writes lost')).toBe(false)
  })
  it('adds stale reads only when some design served any', () => {
    const cols = comparisonColumns([r({ staleReadPct: 0.468 })])
    expect(cols.find((c) => c.label === 'Stale reads')?.format(r({ staleReadPct: 0.468 }))).toBe('46.8%')
  })
  it('formats latencies of seconds readably', () => {
    expect(comparisonColumns([r()])[1].format(r({ p95: 2027 }))).toBe('2.0 s')
  })
})
