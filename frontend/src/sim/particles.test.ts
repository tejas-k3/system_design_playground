import { describe, expect, it } from 'vitest'
import { spawnTimes } from './particles'

describe('spawnTimes', () => {
  it('spreads a burst evenly over the time since the last frame, newest at "now"', () => {
    const t = spawnTimes(4, 1000, 40)
    expect(t).toEqual([970, 980, 990, 1000])
  })
  it('keeps every spawn inside the interval, in order', () => {
    const t = spawnTimes(50, 5000, 40)
    expect(t[0]).toBeGreaterThan(4960)
    expect(t[49]).toBe(5000)
    expect([...t].sort((a, b) => a - b)).toEqual(t)
  })
  it('does not stretch a long gap (a background tab) over the whole gap', () => {
    const t = spawnTimes(2, 10000, 5000)
    expect(t[0]).toBeGreaterThanOrEqual(10000 - 120)
  })
  it('handles an empty burst', () => {
    expect(spawnTimes(0, 1000, 40)).toEqual([])
  })
})
