import { describe, expect, it } from 'vitest'
import type { Alternative } from '../api/types'
import { defendQuestion, selectDefendAlternative } from './defend'

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
