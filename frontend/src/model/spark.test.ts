import { describe, expect, it } from 'vitest'
import { sparkSummary, sparkPoints } from './spark'

describe('sparkPoints', () => {
  it('maps values to pixel coordinates inside the box, newest on the right, larger values higher', () => {
    const pts = sparkPoints([0, 5, 10], { width: 100, height: 20, max: 10 })
    expect(pts).toEqual([[0, 20], [50, 10], [100, 0]])
  })
  it('clamps values above max and below zero', () => {
    expect(sparkPoints([-3, 99], { width: 10, height: 10, max: 10 })).toEqual([[0, 10], [10, 0]])
  })
  it('draws a single point at the left and nothing for no data', () => {
    expect(sparkPoints([4], { width: 10, height: 10, max: 8 })).toEqual([[0, 5]])
    expect(sparkPoints([], { width: 10, height: 10, max: 8 })).toEqual([])
  })
  it('uses the data maximum when none is given, never dividing by zero', () => {
    expect(sparkPoints([2, 4], { width: 10, height: 8 })).toEqual([[0, 4], [10, 0]])
    expect(sparkPoints([0, 0], { width: 10, height: 8 })).toEqual([[0, 8], [10, 8]])
  })
})

describe('sparkSummary', () => {
  it('describes the latest value, the peak and the window in words', () => {
    expect(sparkSummary('Busy', [0.2, 0.9, 0.5], (v) => `${Math.round(v * 100)}%`)).toBe('Busy: now 50%, peak 90%, over the last 3 s.')
  })
  it('says there is no data yet', () => {
    expect(sparkSummary('Busy', [], String)).toBe('Busy: no data yet.')
  })
})
