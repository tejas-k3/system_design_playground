import { describe, expect, it } from 'vitest'
import type { Stats } from '../api/types'
import { emptyStats } from '../state/store'
import { goalRows } from './goals'

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

  it('only lists goals the challenge defines', () => {
    expect(goalRows({ minSuccessRate: 0.9 }, emptyStats(), 0, false).map((r) => r.label)).toEqual(['Success rate'])
  })
})
