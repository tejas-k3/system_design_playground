import { describe, expect, it } from 'vitest'
import type { Alternative } from '../api/types'
import { defendQuestion, forKey, selectDefendAlternative } from './defend'

const alternative = (label: string, pass: boolean): Alternative => ({
  label, pass, changes: [], successPct: 0, p95Ms: 0, costPerHour: 0, why: '',
})

describe('selectDefendAlternative', () => {
  it('picks the first alternative that failed', () => {
    expect(selectDefendAlternative([alternative('a', true), alternative('b', false), alternative('c', false)])).toBe(1)
  })
  it('falls back to the first when every alternative passes', () => {
    expect(selectDefendAlternative([alternative('a', true), alternative('b', true)])).toBe(0)
  })
  it('returns null when there are no alternatives', () => {
    expect(selectDefendAlternative([])).toBeNull()
  })
})

it('formats the counter-question with a lower-cased first letter', () => {
  expect(defendQuestion('Give each server 32 threads')).toBe('Why not give each server 32 threads?')
})

describe('forKey', () => {
  it('returns the value only for the key it was made for', () => {
    const entry = { key: 'retry-storm', value: 7 }
    expect(forKey(entry, 'retry-storm')).toBe(7)
    expect(forKey(entry, 'hot-shard')).toBeNull()
    expect(forKey(null, 'retry-storm')).toBeNull()
  })
  it('compares objects by identity, so an edited design no longer matches an old result', () => {
    const before = { nodes: [], edges: [], notes: [] }
    const edited = { ...before }
    const entry = { key: before, value: 'old numbers' }
    expect(forKey(entry, before)).toBe('old numbers')
    expect(forKey(entry, edited)).toBeNull()
  })
})
