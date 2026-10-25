import { afterEach, describe, expect, it } from 'vitest'
import { loadSummaries, MAX_SUMMARIES, parseSummaries, saveSummaries, SUMMARIES_KEY } from './summaries'

const item = (id: string, extra: object = {}) => ({ id, title: `T ${id}`, difficulty: 'Easy', summary: 's', ...extra })

describe('parseSummaries', () => {
  it('reads a good list and keeps known fields only', () => {
    const r = parseSummaries(JSON.stringify([item('a', { concepts: ['caching', 5], level: 'sde2', evil: 1, configOnly: true })]))
    expect(r).toEqual([{ id: 'a', title: 'T a', difficulty: 'Easy', summary: 's', concepts: ['caching'], level: 'sde2', configOnly: true }])
  })
  it('refuses junk, empties, duplicates, bad entries and drops bad levels', () => {
    expect(parseSummaries(null)).toBeNull()
    expect(parseSummaries('{')).toBeNull()
    expect(parseSummaries('[]')).toBeNull()
    expect(parseSummaries('{"a":1}')).toBeNull()
    expect(parseSummaries(JSON.stringify([item('a'), item('a')]))).toBeNull()
    expect(parseSummaries(JSON.stringify([item('a'), { id: 3 }]))).toBeNull()
    expect(parseSummaries(JSON.stringify([item('')]))).toBeNull()
    expect(parseSummaries(JSON.stringify([item('a', { level: 'god' })]))![0].level).toBeUndefined()
  })
  it('caps the count, the text sizes and the stored bytes', () => {
    expect(parseSummaries(JSON.stringify(Array.from({ length: MAX_SUMMARIES + 1 }, (_, i) => item(`i${i}`))))).toBeNull()
    expect(parseSummaries(JSON.stringify(Array.from({ length: MAX_SUMMARIES }, (_, i) => item(`i${i}`))))).toHaveLength(MAX_SUMMARIES)
    expect(parseSummaries(JSON.stringify([item('a', { title: 'x'.repeat(201) })]))).toBeNull()
    expect(parseSummaries(JSON.stringify([item('a', { summary: 'x'.repeat(600_000) })]))).toBeNull()
    expect(parseSummaries(JSON.stringify([item('a', { concepts: Array.from({ length: 50 }, (_, i) => `c${i}`) })]))![0].concepts).toHaveLength(20)
  })
})

describe('storage', () => {
  afterEach(() => { delete (globalThis as { window?: unknown }).window })
  it('round-trips, and survives missing or throwing storage', () => {
    const mem = new Map<string, string>()
    ;(globalThis as { window?: unknown }).window = { localStorage: { getItem: (k: string) => mem.get(k) ?? null, setItem: (k: string, v: string) => void mem.set(k, v) } }
    saveSummaries([item('a')])
    expect(mem.has(SUMMARIES_KEY)).toBe(true)
    expect(loadSummaries()).toHaveLength(1)
    ;(globalThis as { window?: unknown }).window = { localStorage: { getItem: () => { throw new Error('no') }, setItem: () => { throw new Error('no') } } }
    expect(loadSummaries()).toBeNull()
    expect(() => saveSummaries([item('a')])).not.toThrow()
    delete (globalThis as { window?: unknown }).window
    expect(loadSummaries()).toBeNull()
  })
})
