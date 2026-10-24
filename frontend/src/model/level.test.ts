import { describe, expect, it } from 'vitest'
import type { ScenarioSummary } from '../api/types'
import { dailyForLevel } from './daily'
import { LEVEL_TAG, orderByLevel, parseLevel, readLevel, writeLevel } from './level'
import { skillMap } from './skills'

const mk = (id: string, level: ScenarioSummary['level'], difficulty = 'Easy', concepts = ['caching']): ScenarioSummary => ({ id, title: id, difficulty, summary: '', level, concepts })

describe('parseLevel', () => {
  it('accepts the three levels and "all"; anything else is all', () => {
    expect(parseLevel('sde2')).toBe('sde2')
    expect(parseLevel('all')).toBe('all')
    for (const v of ['SDE2', '', null, undefined, 3, {}, '__proto__']) expect(parseLevel(v)).toBe('all')
  })
  it('tags are text', () => expect(LEVEL_TAG).toEqual({ sde1: 'SDE1', sde2: 'SDE2', sde3: 'SDE3' }))
})

describe('orderByLevel', () => {
  const list = [mk('h2', 'sde2', 'Hard'), mk('e1', 'sde1', 'Easy'), mk('m2', 'sde2', 'Medium'), mk('t1', 'sde1', 'Tutorial'), mk('x', undefined, 'Easy'), mk('e2', 'sde2', 'Easy')]
  it('with All it keeps the list exactly and has no other group', () => {
    const o = orderByLevel(list, 'all')
    expect(o.first).toEqual(list)
    expect(o.other).toEqual([])
  })
  it('puts the chosen level first by difficulty, the rest after; nothing is dropped', () => {
    const o = orderByLevel(list, 'sde2')
    expect(o.first.map((s) => s.id)).toEqual(['e2', 'm2', 'h2'])
    expect(o.other.map((s) => s.id)).toEqual(['t1', 'e1', 'x'])
    expect(o.first.length + o.other.length).toBe(list.length)
  })
  it('is stable inside a difficulty and does not change the input', () => {
    const same = [mk('b', 'sde1'), mk('a', 'sde1'), mk('c', 'sde1')]
    expect(orderByLevel(same, 'sde1').first.map((s) => s.id)).toEqual(['b', 'a', 'c'])
    const copy = [...list]
    orderByLevel(list, 'sde1')
    expect(list).toEqual(copy)
  })
})

describe('dailyForLevel', () => {
  const list = [...'abcdef'].map((c, i) => mk(c, i < 3 ? 'sde1' : 'sde2'))
  const plain = (d: string) => dailyForLevel(d, list.map((s) => ({ ...s, level: undefined })), 'sde1')
  it('All or unknown level behaves exactly like the plain daily challenge', () => {
    for (const d of ['2026-10-20', '2026-10-21', '2027-01-03']) {
      expect(dailyForLevel(d, list, 'all')).toBe(plain(d))
      expect(dailyForLevel(d, list, 'bogus' as never)).toBe(plain(d))
    }
  })
  it('only picks from the chosen level, covers them all and never repeats on consecutive days', () => {
    const seen = new Set<string>()
    let prev = ''
    for (let i = 0; i < 30; i++) {
      const d = new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10)
      const id = dailyForLevel(d, list, 'sde1')!
      expect(['a', 'b', 'c']).toContain(id)
      expect(id).not.toBe(prev)
      prev = id
      if (i < 3) seen.add(id)
    }
    expect(seen.size).toBe(3)
  })
  it('falls back to all challenges when the level has none', () => {
    expect(dailyForLevel('2026-10-20', list, 'sde3')).toBe(plain('2026-10-20'))
  })
})

describe('skillMap next suggestion by level', () => {
  const list = [mk('easy1', 'sde1', 'Easy'), mk('hard3', 'sde3', 'Hard'), mk('med2', 'sde2', 'Medium')]
  it('prefers the chosen level, then falls back to the old rule', () => {
    expect(skillMap(list, {})[0].next).toBe('easy1')
    expect(skillMap(list, {}, 'sde3')[0].next).toBe('hard3')
    expect(skillMap(list, { hard3: 3 }, 'sde3')[0].next).toBe('easy1')
    expect(skillMap(list, {}, 'all')[0].next).toBe('easy1')
  })
})

describe('level storage', () => {
  it('survives a missing storage', () => {
    expect(readLevel()).toBe('all')
    expect(() => writeLevel('sde1')).not.toThrow()
  })
  it('round trips through a storage and ignores junk', () => {
    const mem = new Map<string, string>()
    ;(globalThis as { window?: unknown }).window = { localStorage: { getItem: (k: string) => mem.get(k) ?? null, setItem: (k: string, v: string) => void mem.set(k, v) } }
    try {
      writeLevel('sde3')
      expect(readLevel()).toBe('sde3')
      mem.set('sdp:level', 'junk')
      expect(readLevel()).toBe('all')
    } finally {
      delete (globalThis as { window?: unknown }).window
    }
  })
})
