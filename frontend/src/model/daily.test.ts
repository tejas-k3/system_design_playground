import { describe, expect, it } from 'vitest'
import { dailyChallenge, dayNumber, nextStreak, shownStreak } from './daily'

const ids = ['a', 'b', 'c', 'd', 'e', 'f', 'g']

describe('dailyChallenge', () => {
  it('is the same for the same date and list, whatever order the list arrives in', () => {
    expect(dailyChallenge('2026-10-20', ids)).toBe(dailyChallenge('2026-10-20', [...ids].reverse()))
  })
  it('shows every challenge once before any repeats', () => {
    const start = dayNumber('2026-01-01')
    const seen = new Set<string>()
    for (let i = 0; i < ids.length; i++) seen.add(dailyChallenge(dayNumber2date(start + i), ids)!)
    expect(seen.size).toBe(ids.length)
  })
  it('does not repeat on two days in a row', () => {
    const start = dayNumber('2026-03-01')
    for (let i = 0; i < 40; i++) expect(dailyChallenge(dayNumber2date(start + i), ids)).not.toBe(dailyChallenge(dayNumber2date(start + i + 1), ids))
  })
  it('keeps both promises over a year for several list sizes', () => {
    for (const n of [3, 4, 5, 7, 11, 40]) {
      const list = Array.from({ length: n }, (_, i) => `s${i}`)
      const start = dayNumber('2026-01-01')
      let prev = ''
      for (let i = 0; i < 400; i++) {
        const cur = dailyChallenge(dayNumber2date(start + i), list)!
        expect(cur).not.toBe(prev)
        prev = cur
      }
      const counts = new Map<string, number>()
      for (let i = 0; i < n * 10; i++) { const c = dailyChallenge(dayNumber2date(start + i), list)!; counts.set(c, (counts.get(c) ?? 0) + 1) }
      expect([...counts.values()].every((v) => v >= 9 && v <= 11)).toBe(true)
    }
  })
  it('has nothing to show for an empty list', () => expect(dailyChallenge('2026-10-20', [])).toBeUndefined())
  it('rejects a malformed date', () => expect(dailyChallenge('yesterday', ids)).toBeUndefined())
})

describe('streak', () => {
  it('starts at 1 and grows on consecutive days', () => {
    let s = nextStreak(undefined, '2026-10-20')
    expect(s).toEqual({ last: '2026-10-20', count: 1 })
    s = nextStreak(s, '2026-10-21')
    expect(s.count).toBe(2)
  })
  it('does not count the same day twice', () => {
    const s = { last: '2026-10-20', count: 3 }
    expect(nextStreak(s, '2026-10-20')).toEqual(s)
  })
  it('restarts after a missed day, including across a month end', () => {
    expect(nextStreak({ last: '2026-10-18', count: 5 }, '2026-10-20').count).toBe(1)
    expect(nextStreak({ last: '2026-10-31', count: 2 }, '2026-11-01').count).toBe(3)
  })
  it('shows a streak only while it is still alive', () => {
    const s = { last: '2026-10-20', count: 4 }
    expect(shownStreak(s, '2026-10-20')).toBe(4)
    expect(shownStreak(s, '2026-10-21')).toBe(4) // yesterday: still can be continued today
    expect(shownStreak(s, '2026-10-22')).toBe(0)
    expect(shownStreak(undefined, '2026-10-22')).toBe(0)
  })
})

function dayNumber2date(n: number): string {
  return new Date(n * 86400000).toISOString().slice(0, 10)
}
