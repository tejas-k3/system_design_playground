import { describe, expect, it } from 'vitest'
import { emptyInterview, MAX_ROWS, sanitizeInterview } from './interview'
import { newSession } from './timer'

const roundTrip = (v: unknown) => sanitizeInterview(JSON.parse(JSON.stringify(v)))

describe('sanitizeInterview', () => {
  it('accepts what it produced', () => {
    const d = emptyInterview()
    expect(roundTrip(d)).toEqual(d)
    const full = { ...emptyInterview(), session: newSession(45 * 60_000, 1_700_000_000_000), functional: ['post a photo'], nonFunctional: [{ label: 'availability', target: 99.9, unit: '%' }], api: [{ method: 'POST', path: '/photos', request: '{}', response: '201', notes: '' }], tables: [{ name: 'photos', fields: 'id, owner', key: 'id', indexes: 'owner', access: 'by owner' }], rubric: { scalability: 4, availability: 3, consistency: 0, cost: 2, communication: 5 }, notes: 'x' }
    expect(roundTrip(full)).toEqual(full)
  })
  it('refuses things that are not an interview, whole', () => {
    for (const bad of [null, 5, 'x', [], {}, { ...emptyInterview(), api: 'no' }, { ...emptyInterview(), rubric: null }, { ...emptyInterview(), estimate: 3 }]) expect(sanitizeInterview(bad)).toBeUndefined()
  })
  it('refuses a list with a malformed row instead of keeping part of it', () => {
    expect(sanitizeInterview({ ...emptyInterview(), api: [{ method: 'GET', path: '/a', request: '', response: '', notes: '' }, { method: 5 }] })).toBeUndefined()
    expect(sanitizeInterview({ ...emptyInterview(), nonFunctional: [{ label: 'x', target: 'high', unit: '' }] })).toBeUndefined()
  })
  it('clamps numbers and caps text and list length', () => {
    const d = emptyInterview()
    const wild = { ...d, estimate: { ...d.estimate, dauPct: 900, headroomPct: 200, peakFactor: -3, perServerQps: 0 }, rubric: { ...d.rubric, scalability: 99 }, functional: Array.from({ length: 100 }, () => 'x'.repeat(1000)) }
    const out = sanitizeInterview(wild)!
    expect(out.estimate.dauPct).toBe(100)
    expect(out.estimate.headroomPct).toBe(95)
    expect(out.estimate.peakFactor).toBe(1)
    expect(out.estimate.perServerQps).toBe(1)
    expect(out.rubric.scalability).toBe(5)
    expect(out.functional).toHaveLength(MAX_ROWS)
    expect(out.functional[0]).toHaveLength(200)
  })
  it('drops fields it does not know', () => {
    const out = sanitizeInterview({ ...emptyInterview(), evil: '<script>', estimate: { ...emptyInterview().estimate, extra: 1 } })!
    expect(out).not.toHaveProperty('evil')
    expect(out.estimate).not.toHaveProperty('extra')
  })
  it('refuses a damaged timer session but not a missing one', () => {
    expect(sanitizeInterview({ ...emptyInterview(), session: null })).toBeDefined()
    expect(sanitizeInterview({ ...emptyInterview(), session: { ...newSession(45 * 60_000, 1), stage: 'bogus' } })).toBeUndefined()
    expect(sanitizeInterview({ ...emptyInterview(), session: { ...newSession(45 * 60_000, 1), switches: [] } })).toBeUndefined()
    expect(sanitizeInterview({ ...emptyInterview(), session: { ...newSession(45 * 60_000, 1), totalMs: 'long' } })).toBeUndefined()
  })
})
