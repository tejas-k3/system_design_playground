import { describe, expect, it } from 'vitest'
import type { SweepPoint } from '../api/types'
import { describeCurve, findKnee } from './sweep'

const p = (rps: number, successRate: number, p95: number): SweepPoint => ({ value: rps / 10, rps, successRate, p50: p95 / 2, p95, p99: p95 * 1.2, costPerHour: 1 })

describe('findKnee', () => {
  it('is the first load where success drops under 99 %', () => {
    const pts = [p(20, 1, 100), p(40, 1, 110), p(80, 0.97, 150), p(160, 0.5, 600)]
    expect(findKnee(pts)).toBe(2)
  })
  it('is the first load where p95 reaches three times the lightest load', () => {
    const pts = [p(20, 1, 100), p(40, 1, 200), p(80, 1, 310), p(160, 1, 900)]
    expect(findKnee(pts)).toBe(2)
  })
  it('takes whichever rule trips first', () => {
    expect(findKnee([p(20, 1, 100), p(40, 0.9, 120), p(80, 0.8, 900)])).toBe(1)
  })
  it('is null when the design never bends in the range, or there are too few points', () => {
    expect(findKnee([p(20, 1, 100), p(40, 1, 120), p(80, 1, 150)])).toBeNull()
    expect(findKnee([p(20, 1, 100)])).toBeNull()
    expect(findKnee([])).toBeNull()
  })
  it('never calls the lightest point the knee', () => {
    expect(findKnee([p(20, 0.5, 900), p(40, 0.4, 900)])).toBe(1)
  })
})

describe('describeCurve', () => {
  it('states the last comfortable load and the knee in words', () => {
    const pts = [p(20, 1, 100), p(80, 0.97, 150)]
    expect(describeCurve(pts, 1)).toBe('Comfortable up to 20 requests per second; the knee is at 80 (success 97.0%, p95 150 ms).')
  })
  it('says so when there is no knee', () => {
    expect(describeCurve([p(20, 1, 100), p(80, 1, 120)], null)).toBe('No knee up to 80 requests per second.')
  })
  it('handles an empty result', () => {
    expect(describeCurve([], null)).toBe('No data yet.')
  })
})
