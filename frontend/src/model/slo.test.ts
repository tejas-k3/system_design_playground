import { describe, expect, it } from 'vitest'
import type { SeriesPoint } from '../api/types'
import { errorBudget } from './slo'

const pt = (t: number, ok: number, err: number): SeriesPoint => ({ t, okRps: ok, errRps: err, p50: 0, p95: 0, successRate: 1, inFlight: 0 })
const flat = (from: number, to: number, ok: number, err: number) => Array.from({ length: to - from + 1 }, (_, i) => pt(from + i, ok, err))

describe('errorBudget', () => {
  it('allows 10 failures in 10,000 requests at 99.9 %', () => {
    const b = errorBudget({ resolved: 10000, failed: 4 }, [], 99.9)
    expect(b.allowed).toBeCloseTo(10, 6)
    expect(b.used).toBe(4)
    expect(b.remaining).toBeCloseTo(6, 6)
    expect(b.remainingFrac).toBeCloseTo(0.6, 6)
    expect(b.status).toBe('ok')
  })
  it('goes negative but reports a clamped fraction once the budget is spent', () => {
    const b = errorBudget({ resolved: 10000, failed: 25 }, [], 99.9)
    expect(b.remaining).toBeCloseTo(-15, 6)
    expect(b.remainingFrac).toBe(0)
    expect(b.status).toBe('exhausted')
  })
  it('has the whole budget before any request resolves', () => {
    const b = errorBudget({ resolved: 0, failed: 0 }, [], 99.9)
    expect(b.remainingFrac).toBe(1)
    expect(b.status).toBe('ok')
  })
  it('burn rate 1.0 means failures arrive exactly at the allowed rate', () => {
    const b = errorBudget({ resolved: 1000, failed: 1 }, flat(0, 99, 999, 1), 99.9)
    expect(b.burnFast).toBeCloseTo(1, 6)
    expect(b.burnSlow).toBeCloseTo(1, 6)
  })
  it('a fresh outage burns fast in the short window long before the long window notices', () => {
    const series = [...flat(0, 49, 100, 0), ...flat(50, 59, 0, 100)] // 50 s healthy, then 10 s of total failure
    const b = errorBudget({ resolved: 6000, failed: 1000 }, series, 99.9)
    expect(b.burnFast).toBeCloseTo(1000, 3)
    expect(b.burnSlow).toBeLessThan(b.burnFast)
    expect(b.burnSlow).toBeGreaterThan(100)
  })
  it('reports zero burn when there is no traffic in the window', () => {
    expect(errorBudget({ resolved: 0, failed: 0 }, flat(0, 20, 0, 0), 99.9)).toMatchObject({ burnFast: 0, burnSlow: 0 })
  })
  it('marks a burn above 14x as burning while budget remains', () => {
    const b = errorBudget({ resolved: 100000, failed: 20 }, flat(0, 30, 0, 0).map((p, i) => (i > 20 ? pt(p.t, 90, 10) : p)), 99.9)
    expect(b.status).toBe('burning')
  })
})
