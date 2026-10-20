import { describe, expect, it } from 'vitest'
import type { ScenarioSummary } from '../api/types'
import { skillMap } from './skills'

const sc = (id: string, difficulty: string, concepts?: string[]): ScenarioSummary => ({ id, title: id, difficulty, summary: '', concepts })

describe('skillMap', () => {
  const list = [sc('a', 'Hard', ['caching']), sc('b', 'Easy', ['caching', 'queues']), sc('c', 'Medium', ['queues']), sc('d', 'Easy')]

  it('has one row for each concept in use, in the standard order, and ignores untagged challenges', () => {
    const rows = skillMap(list, {})
    expect(rows.map((r) => r.concept)).toEqual(['caching', 'queues'])
    expect(rows[0]).toMatchObject({ total: 2, stars: 0, maxStars: 6, solved: 0 })
  })
  it('counts stars and solved challenges', () => {
    const rows = skillMap(list, { a: 3, b: 1 })
    expect(rows[0]).toMatchObject({ stars: 4, solved: 2 })
    expect(rows[1]).toMatchObject({ stars: 1, solved: 1 })
  })
  it('recommends the lowest-star challenge, the easiest first on a tie, and nothing once all are mastered', () => {
    expect(skillMap(list, {})[0].next).toBe('b')
    expect(skillMap(list, { b: 2 })[0].next).toBe('a')
    expect(skillMap(list, { a: 3, b: 3 })[0].next).toBeUndefined()
  })
  it('does not recommend a challenge that is already at three stars while another is not', () => {
    expect(skillMap(list, { b: 3 })[1].next).toBe('c')
  })
  it('puts a concept it does not know after the standard ones', () => {
    const rows = skillMap([sc('x', 'Easy', ['zzz', 'caching'])], {})
    expect(rows.map((r) => r.concept)).toEqual(['caching', 'zzz'])
  })
})
