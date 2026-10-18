import { describe, expect, it } from 'vitest'
import type { SeriesPoint } from '../api/types'
import { describeSeries } from './symptoms'

const pt = (t: number, v: number): SeriesPoint => ({ t, okRps: v, errRps: 0, p50: 0, p95: 0, successRate: 1, inFlight: 0 })

describe('describeSeries', () => {
  it('names the start, the low and the high with when they happened, and the end', () => {
    const s = [pt(0, 100), pt(4, 120), pt(8, 0), pt(12, 110)]
    expect(describeSeries('Throughput', s, (p) => p.okRps, 'req/s')).toBe('Throughput, req/s. Starts at 100, peaks at 120 (4 s), lowest 0 (8 s), ends at 110.')
  })
  it('says when it never changes', () => {
    expect(describeSeries('p95', [pt(0, 50), pt(2, 50)], (p) => p.okRps, 'ms')).toBe('p95, ms. Flat at 50.')
  })
  it('handles no data', () => {
    expect(describeSeries('x', [], (p) => p.okRps, 'ms')).toBe('x: no data.')
  })
})
