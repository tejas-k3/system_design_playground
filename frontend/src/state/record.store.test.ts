import { beforeEach, describe, expect, it } from 'vitest'
import type { Scenario, Score } from '../api/types'
import { parseSaved } from './persist'
import { reg } from '../test/fixtures'
import { useStore } from './store'

const st = () => useStore.getState()

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
})

const def = { id: 's1', title: 'T', difficulty: 'Easy', summary: '', story: '', speed: 1, durationSec: 10, warmupSec: 0, goals: {}, hintCount: 3, diagram: { nodes: [], edges: [], notes: [] } } as unknown as Scenario
const pred = { id: 'p1', question: 'q', options: ['a', 'b', 'c'] }
const score = (pass: boolean): Score => ({ pass, stars: pass ? 2 : 0, checks: [] }) as unknown as Score
const ans = (correct: boolean) => ({ choice: 0, correct, answer: 1, explain: '' })

describe('level setting', () => {
  it('defaults to all and is changed by setLevel', () => {
    expect(st().level).toBe('all')
    st().setLevel('sde2')
    expect(st().level).toBe('sde2')
  })
})

describe('challenge records', () => {
  it('counts a wrong prediction but not a right one', () => {
    st().openScenario(def)
    st().recordPrediction(pred, ans(false))
    expect(st().records.s1.wrong).toBe(1)
    st().recordPrediction(pred, ans(false)) // already answered: ignored
    expect(st().records.s1.wrong).toBe(1)
  })
  it('keeps the highest hint and flags the pass as late once hint 3 was seen', () => {
    st().openScenario(def)
    st().addHint('h1')
    st().addHint('h2')
    st().setResult(score(true))
    expect(st().records.s1).toMatchObject({ hints: 2, runs: 1, passed: true, earlyPass: true })
    st().openScenario({ ...def, id: 's2' })
    st().addHint('1')
    st().addHint('2')
    st().addHint('3')
    st().setResult(score(false))
    st().setResult(score(true))
    expect(st().records.s2).toMatchObject({ hints: 3, runs: 2, passed: true, earlyPass: false })
  })
  it('records that the answer was used', () => {
    st().openScenario(def)
    st().showAnswer('Run 5')
    expect(st().scenario!.answer).toBe('Run 5')
    expect(st().records.s1.answerUsed).toBe(true)
  })
  it('survives a damaged save and drops hostile entries', () => {
    const saved = JSON.stringify({ records: { ok: { wrong: 1, hints: 1, runs: 1, passed: true, earlyPass: true, answerUsed: false }, bad: { wrong: 'x' }, '__proto__': { wrong: 1 } } })
    expect(Object.keys(parseSaved(saved, reg).records ?? {})).toEqual(['ok'])
    expect(parseSaved(JSON.stringify({ records: [1, 2] }), reg).records).toBeUndefined()
  })
})

describe('debrief and deeper', () => {
  it('keeps a non-empty deeper text and treats a missing or blank one as none', () => {
    st().openScenario(def)
    st().setDebrief('d', 'more')
    expect(st().scenario).toMatchObject({ debrief: 'd', deeper: 'more' })
    st().setDebrief('d', '   ')
    expect(st().scenario!.deeper).toBeNull()
    st().setDebrief('d')
    expect(st().scenario!.deeper).toBeNull()
  })
})
