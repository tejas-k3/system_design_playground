import { describe, expect, it } from 'vitest'
import type { ScenarioSummary } from '../api/types'
import { exportProgress, mergeProgress, parseImport, MAX_ENTRIES } from './progressIO'
import { emptyRecord, needsReview, noteAnswer, noteHint, noteRun, noteWrong, reviewReason } from './record'
import { skillMap } from './skills'

describe('record keeping', () => {
  it('counts wrong predictions', () => {
    expect(noteWrong(noteWrong(undefined)).wrong).toBe(2)
  })
  it('keeps the highest hint only', () => {
    let r = noteHint(undefined, 2)
    r = noteHint(r, 1)
    expect(r.hints).toBe(2)
    expect(noteHint(r, 3).hints).toBe(3)
  })
  it('counts attempts up to the first pass and then stops', () => {
    let r = noteRun(undefined, false)
    r = noteRun(r, false)
    r = noteRun(r, true)
    expect(r).toMatchObject({ runs: 3, passed: true, earlyPass: true })
    expect(noteRun(r, false)).toEqual(r)
    expect(noteRun(r, true).runs).toBe(3)
  })
  it('a pass after hint 3 was revealed is not early, and stays so', () => {
    let r = noteHint(undefined, 3)
    r = noteRun(r, true)
    expect(r.earlyPass).toBe(false)
  })
  it('a pass after hint 2 only is early', () => {
    expect(noteRun(noteHint(undefined, 2), true).earlyPass).toBe(true)
  })
  it('records that the answer was used', () => expect(noteAnswer(undefined).answerUsed).toBe(true))
  it('caps counters', () => {
    let r = emptyRecord()
    for (let i = 0; i < 1100; i++) r = noteWrong(r)
    expect(r.wrong).toBe(1000)
  })
})

describe('review rule', () => {
  const passed = { ...emptyRecord(), passed: true, runs: 1, earlyPass: true }
  it('needs review when passed and the answer was used or a prediction was wrong', () => {
    expect(needsReview({ ...passed, answerUsed: true })).toBe(true)
    expect(needsReview({ ...passed, wrong: 1 })).toBe(true)
  })
  it('does not for a clean pass, an unpassed challenge, hints alone, or no record', () => {
    expect(needsReview(passed)).toBe(false)
    expect(needsReview({ ...passed, hints: 3, runs: 5 })).toBe(false)
    expect(needsReview({ ...emptyRecord(), wrong: 2, answerUsed: true })).toBe(false)
    expect(needsReview(undefined)).toBe(false)
  })
  it('explains itself in words', () => {
    expect(reviewReason({ ...passed, answerUsed: true, wrong: 2 })).toBe('the answer was shown and 2 predictions were wrong')
    expect(reviewReason({ ...passed, wrong: 1 })).toBe('1 prediction was wrong')
  })
  it('marks a concept when any one of its challenges needs review', () => {
    const list: ScenarioSummary[] = [
      { id: 'a', title: 'a', difficulty: 'Easy', summary: '', concepts: ['caching'] },
      { id: 'b', title: 'b', difficulty: 'Easy', summary: '', concepts: ['caching', 'capacity'] },
      { id: 'c', title: 'c', difficulty: 'Easy', summary: '', concepts: ['capacity'] },
    ]
    const rows = skillMap(list, { a: 3, b: 3 }, 'all', { a: passed, b: { ...passed, answerUsed: true } })
    expect(rows.find((r) => r.concept === 'caching')!.review).toEqual(['b'])
    expect(rows.find((r) => r.concept === 'capacity')!.review).toEqual(['b'])
    expect(skillMap(list, {}, 'all', {}).every((r) => r.review.length === 0)).toBe(true)
  })
})

describe('export and import', () => {
  const rec = { ...emptyRecord(), wrong: 1, hints: 3, runs: 4, passed: true, earlyPass: false, answerUsed: true }
  const good = { progress: { 'hello-world': 3, 'slow-reads': 1 }, records: { 'slow-reads': rec } }
  it('round trips', () => {
    const r = parseImport(exportProgress(good))
    expect(r).toEqual({ ok: true, skipped: 0, ...good })
  })
  it('ignores unknown fields everywhere', () => {
    const t = JSON.stringify({ format: 'sdp-progress', version: 1, evil: 1, progress: { a: 2 }, records: { a: { ...rec, extra: 'x' } } })
    const r = parseImport(t)
    expect(r.ok && r.records.a).toEqual(rec)
    expect(r.ok && Object.keys(r.records.a)).not.toContain('extra')
  })
  it('rejects non-JSON, wrong shapes and wrong format', () => {
    for (const t of ['', 'nope', '[]', 'null', '"x"', '{}', JSON.stringify({ format: 'other', version: 1, progress: {} }), JSON.stringify({ format: 'sdp-progress', version: 2, progress: {} }), JSON.stringify({ format: 'sdp-progress', version: 1, progress: [] }), JSON.stringify({ format: 'sdp-progress', version: 1, progress: {}, records: 5 })]) {
      expect(parseImport(t).ok).toBe(false)
    }
  })
  it('skips entries with wrong types or out of range values', () => {
    const t = JSON.stringify({ format: 'sdp-progress', version: 1, progress: { ok: 2, s: '3', f: 1.5, neg: -1, big: 4, nul: null }, records: { r1: { ...rec, wrong: '1' }, r2: { ...rec, hints: 99 }, r3: { ...rec, passed: 'yes' }, r4: [], r5: rec } })
    const r = parseImport(t)
    expect(r.ok && r.progress).toEqual({ ok: 2 })
    expect(r.ok && Object.keys(r.records)).toEqual(['r5'])
    expect(r.ok && r.skipped).toBe(9)
  })
  it('refuses huge files and huge maps', () => {
    expect(parseImport('x'.repeat(300_000)).ok).toBe(false)
    const progress: Record<string, number> = {}
    for (let i = 0; i <= MAX_ENTRIES; i++) progress[`c${i}`] = 1
    expect(parseImport(JSON.stringify({ format: 'sdp-progress', version: 1, progress })).ok).toBe(false)
  })
  it('does not take prototype keys or odd ids, and pollutes nothing', () => {
    const t = '{"format":"sdp-progress","version":1,"progress":{"__proto__":3,"constructor":2,"prototype":1,"ok":1,"Bad Id":1,"a/b":1},"records":{"__proto__":{"wrong":1,"hints":1,"runs":1,"passed":true,"earlyPass":true,"answerUsed":true}}}'
    const r = parseImport(t)
    expect(r.ok && r.progress).toEqual({ ok: 1 })
    expect(r.ok && r.records).toEqual({})
    expect(({} as Record<string, unknown>).wrong).toBeUndefined()
    expect(Object.getPrototypeOf(r.ok ? r.progress : {})).toBe(Object.prototype)
  })
  it('a huge array in place of a map is an error, not a crash', () => {
    const t = JSON.stringify({ format: 'sdp-progress', version: 1, progress: Array.from({ length: 50_000 }, () => 1) })
    expect(parseImport(t).ok).toBe(false)
  })
  it('merges: higher stars win, imported notes replace', () => {
    const m = mergeProgress({ progress: { a: 3, b: 1 }, records: { a: rec } }, { progress: { a: 1, b: 2, c: 1 }, records: { b: rec } })
    expect(m.progress).toEqual({ a: 3, b: 2, c: 1 })
    expect(Object.keys(m.records).sort()).toEqual(['a', 'b'])
  })
})
