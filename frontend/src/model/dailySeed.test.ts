import { describe, expect, it } from 'vitest'
import { dailySeed, dayNumber } from './daily'

const toDate = (n: number) => new Date(n * 86400000).toISOString().slice(0, 10)
const dates = (n: number, from = '2026-01-01') => Array.from({ length: n }, (_, i) => toDate(dayNumber(from) + i))

describe('dailySeed', () => {
  it('is the same for the same date', () => {
    expect(dailySeed('2026-10-25')).toBe(dailySeed('2026-10-25'))
  })
  it('stays inside the verified seeds 1 to 8, whole numbers', () => {
    for (const d of dates(800)) {
      const s = dailySeed(d)
      expect(Number.isInteger(s) && s >= 1 && s <= 8).toBe(true)
    }
  })
  it('differs on consecutive days for at least 6 of any 7', () => {
    for (let start = 0; start < 400; start++) {
      const ds = dates(8, toDate(dayNumber('2026-01-01') + start))
      let differ = 0
      for (let i = 0; i < 7; i++) if (dailySeed(ds[i]) !== dailySeed(ds[i + 1])) differ++
      expect(differ).toBeGreaterThanOrEqual(6)
    }
  })
  it('uses the whole range over 400 days', () => {
    expect(new Set(dates(400).map(dailySeed))).toEqual(new Set([1, 2, 3, 4, 5, 6, 7, 8]))
  })
  it('falls back to a valid seed for a bad date', () => {
    expect(dailySeed('nonsense')).toBe(1)
  })
})
