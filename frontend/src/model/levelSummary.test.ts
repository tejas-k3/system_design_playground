import { describe, expect, it } from 'vitest'
import type { Level, ScenarioSummary } from '../api/types'
import { levelSummary } from './skills'

const sc = (id: string, level?: Level): ScenarioSummary => ({ id, title: id, difficulty: 'Easy', summary: '', level })

describe('levelSummary', () => {
  const list = [sc('a', 'sde1'), sc('b', 'sde1'), sc('c', 'sde2'), sc('d', 'sde3'), sc('e', 'sde3'), sc('f')]

  it('has SDE1, SDE2 and SDE3 in order, with stars out of three for each challenge', () => {
    const rows = levelSummary(list, { a: 3, b: 1, d: 2 })
    expect(rows.map((r) => r.label)).toEqual(['SDE1', 'SDE2', 'SDE3'])
    expect(rows[0]).toMatchObject({ level: 'sde1', total: 2, solved: 2, stars: 4, maxStars: 6 })
    expect(rows[1]).toMatchObject({ total: 1, solved: 0, stars: 0, maxStars: 3 })
    expect(rows[2]).toMatchObject({ total: 2, solved: 1, stars: 2, maxStars: 6 })
  })
  it('leaves out challenges with no level and ignores stars for unknown ids; caps stars at three', () => {
    const rows = levelSummary(list, { f: 3, zzz: 3, c: 9 })
    expect(rows.reduce((n, r) => n + r.total, 0)).toBe(5)
    expect(rows.reduce((n, r) => n + r.stars, 0)).toBe(3)
  })
  it('keeps a level row even with nothing in it, and never divides by zero', () => {
    const rows = levelSummary([sc('a', 'sde1')], {})
    expect(rows).toHaveLength(3)
    expect(rows[2]).toMatchObject({ total: 0, maxStars: 0, stars: 0 })
  })
  it('gives a text label with the numbers', () => {
    expect(levelSummary(list, { a: 3 })[0].text).toBe('1 of 2 challenges solved · 3 of 6 stars')
  })
})
