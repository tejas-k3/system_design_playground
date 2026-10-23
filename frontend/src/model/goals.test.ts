import { describe, expect, it } from 'vitest'
import type { Stats } from '../api/types'
import { emptyStats } from '../state/store'
import { failedAfter, fmtCost, goalCost, goalRows, starTargets, successRate } from './goals'

const stats = (run: Partial<Stats['global']['run']>): Stats => ({ ...emptyStats(), global: { now: 5, inFlight: 0, cost: 0, avgCost: 0, run: { ...emptyStats().global.run, ...run } } })
const row = (rows: ReturnType<typeof goalRows>, label: string) => rows.find((r) => r.label === label)!

describe('per-client guarantees goal', () => {
  const goals = { minSuccessRate: 0.9, maxSessionViolations: 0 }
  it('adds up own-write misses and backwards reads', () => {
    const r = row(goalRows(goals, stats({ resolved: 100, ok: 100, readYourWritesBroken: 3, monotonicBroken: 2 }), 1, true), 'Per-client guarantees broken')
    expect(r.actual).toBe('5')
    expect(r.state).toBe('fail')
  })
  it('passes with none', () => {
    expect(row(goalRows(goals, stats({ resolved: 100, ok: 100 }), 1, true), 'Per-client guarantees broken').state).toBe('pass')
  })
})

describe('recovery goals', () => {
  const goals = { minSuccessRate: 0.5, maxRpoSec: 10, maxRtoSec: 30 }
  it('compares the data lost and the time to restore with their limits', () => {
    const rows = goalRows(goals, stats({ resolved: 10, ok: 10, rpoSec: 8, rtoSec: 51.2 }), 1, true)
    expect(row(rows, 'Data lost (RPO)')).toMatchObject({ actual: '8.0 s', state: 'pass' })
    expect(row(rows, 'Time to restore (RTO)')).toMatchObject({ actual: '51.2 s', state: 'fail' })
  })
})

describe('events lost goal', () => {
  it('compares the lost events with the limit', () => {
    const g = { minSuccessRate: 0.5, maxLostEvents: 0 }
    expect(row(goalRows(g, stats({ resolved: 10, ok: 10, eventsLost: 1020 }), 1, true), 'Events lost')).toMatchObject({ actual: '1020', state: 'fail' })
    expect(row(goalRows(g, stats({ resolved: 10, ok: 10 }), 1, true), 'Events lost').state).toBe('pass')
  })
})

describe('quiet tenant goal', () => {
  const g = { minSuccessRate: 0.5, minWorstTenant: 0.95 }
  it('shows the worst-served quiet tenant against the target', () => {
    const rows = goalRows(g, stats({ resolved: 10, ok: 10, tenants: 4, worstTenantSuccess: 0.32 }), 1, true)
    expect(row(rows, 'Worst-served quiet tenant')).toMatchObject({ actual: '32.0%', state: 'fail' })
    expect(row(goalRows(g, stats({ resolved: 10, ok: 10, tenants: 4, worstTenantSuccess: 1 }), 1, true), 'Worst-served quiet tenant').state).toBe('pass')
  })
})

describe('deliveries per message goal', () => {
  const g = { minSuccessRate: 0.9, minDeliveriesPerMessage: 2.9 }
  it('divides the deliveries by the accepted messages', () => {
    const one = goalRows(g, stats({ resolved: 100, ok: 100, jobs: { enqueued: 100, completed: 100 } as never }), 1, true)
    expect(row(one, 'Deliveries per message')).toMatchObject({ actual: '1.00', state: 'fail' })
    const three = goalRows(g, stats({ resolved: 100, ok: 100, jobs: { enqueued: 300, completed: 300 } as never }), 1, true)
    expect(row(three, 'Deliveries per message')).toMatchObject({ actual: '3.00', state: 'pass' })
  })
})

describe('orders left half done goal', () => {
  it('counts the multi-step writes that stayed half done', () => {
    const g = { minSuccessRate: 0.5, maxInconsistent: 0 }
    expect(row(goalRows(g, stats({ resolved: 10, ok: 10, inconsistent: 7 }), 1, true), 'Orders left half done')).toMatchObject({ actual: '7', state: 'fail' })
    expect(row(goalRows(g, stats({ resolved: 10, ok: 10 }), 1, true), 'Orders left half done').state).toBe('pass')
  })
})

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

describe('goalCost', () => {
  const withAvg = (avg: number) => ({ ...emptyStats(), global: { ...emptyStats().global, avgCost: avg } })
  it('is the run average while a run is going, because that is what gets scored', () => {
    expect(goalCost(1.02, withAvg(0.66), true)).toBe(0.66)
  })
  it('is the design cost before a run, or when no average exists yet', () => {
    expect(goalCost(0.5, withAvg(0.66), false)).toBe(0.5)
    expect(goalCost(0.5, withAvg(0), true)).toBe(0.5)
    expect(goalCost(null, withAvg(0), true)).toBeNull()
  })
})
