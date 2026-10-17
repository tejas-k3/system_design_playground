import { describe, expect, it } from 'vitest'
import type { Stats } from '../api/types'
import { emptyStats } from '../state/store'
import { failedAfter, fmtCost, goalRows, starTargets, successRate } from './goals'

const stats = (run: Partial<Stats['global']['run']>): Stats => ({ ...emptyStats(), global: { now: 5, inFlight: 0, cost: 0, run: { ...emptyStats().global.run, ...run } } })
const row = (rows: ReturnType<typeof goalRows>, label: string) => rows.find((r) => r.label === label)!

describe('goalRows', () => {
  const goals = { minSuccessRate: 0.99, maxP95Ms: 500, maxCostPerHour: 1, maxConflicts: 0 }

  it('is pending before a run starts, except for cost which is known up front', () => {
    const rows = goalRows(goals, emptyStats(), 0.4, false)
    expect(row(rows, 'Success rate').state).toBe('pending')
    expect(row(rows, 'p95 latency').state).toBe('pending')
    expect(row(rows, 'Cost').state).toBe('pass')
  })

  it('passes and fails against live numbers', () => {
    const good = goalRows(goals, stats({ resolved: 1000, ok: 995, p95: 300 }), 0.8, true)
    expect(row(good, 'Success rate').state).toBe('pass')
    expect(row(good, 'p95 latency').state).toBe('pass')
    expect(row(good, 'Double bookings').state).toBe('pass')
    const bad = goalRows(goals, stats({ resolved: 1000, ok: 900, p95: 900, conflicts: 2 }), 1.5, true)
    expect(['Success rate', 'p95 latency', 'Double bookings', 'Cost'].map((l) => row(bad, l).state)).toEqual(['fail', 'fail', 'fail', 'fail'])
  })

  it('tracks writes lost in a crash against its limit', () => {
    const g = { minSuccessRate: 0.9, maxLostWrites: 400 }
    expect(row(goalRows(g, stats({ lostWrites: 226 }), 0, true), 'Writes lost in a crash')).toMatchObject({ state: 'pass', actual: '226' })
    expect(row(goalRows(g, stats({ lostWrites: 4777 }), 0, true), 'Writes lost in a crash').state).toBe('fail')
    expect(row(goalRows(g, emptyStats(), 0, false), 'Writes lost in a crash').state).toBe('pending')
  })

  it('only lists goals the challenge defines', () => {
    expect(goalRows({ minSuccessRate: 0.9 }, emptyStats(), 0, false).map((r) => r.label)).toEqual(['Success rate'])
  })
})

describe('starTargets', () => {
  it('spells out what each extra star needs', () => {
    expect(starTargets({ minSuccessRate: 0.99, starCostPerHour: 0.48, starP95Ms: 240 })).toBe('Stars: 2 need cost ≤ $0.48/hr, 3 also need p95 ≤ 240 ms.')
  })
  it('names only the targets a challenge has', () => {
    expect(starTargets({ minSuccessRate: 0.99, starP95Ms: 240 })).toBe('Stars: 3 need p95 ≤ 240 ms.')
    expect(starTargets({ minSuccessRate: 0.99, starCostPerHour: 1.05 })).toBe('Stars: 2 need cost ≤ $1.05/hr.')
    expect(starTargets({ minSuccessRate: 0.99 })).toBe('')
  })
})

describe('successRate', () => {
  it('counts finished requests only, so a request still in flight is neither a success nor a failure yet', () => {
    expect(successRate({ ...emptyStats().global.run, resolved: 80, ok: 60, pending: 20 })).toBe(0.75)
    expect(successRate(emptyStats().global.run)).toBeNull()
  })
  it('is what the success goal reports', () => {
    const run = { resolved: 80, ok: 60, pending: 20 }
    expect(row(goalRows({ minSuccessRate: 0.7 }, stats(run), 0, true), 'Success rate').actual).toBe('75.0%')
  })
})

describe('unknown cost', () => {
  it('is neither a pass nor a fail until the server has priced the design', () => {
    const rows = goalRows({ minSuccessRate: 0.99, maxCostPerHour: 0.6 }, emptyStats(), null, false)
    expect(row(rows, 'Cost').state).toBe('pending')
    expect(row(rows, 'Cost').actual).toBe('—')
  })
  it('is written as a dash, never as $0.00', () => {
    expect(fmtCost(null)).toBe('—')
    expect(fmtCost(0.5)).toBe('$0.50/hr')
  })
})

describe('failedAfter', () => {
  it('is the median time to failure, or a dash when nothing has failed', () => {
    expect(failedAfter({ ...emptyStats().global.run, failed: 3, failP50: 812.4 })).toBe('812 ms')
    expect(failedAfter({ ...emptyStats().global.run, failed: 0, failP50: 0 })).toBe('—')
  })
})
