import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Scenario, Score } from '../api/types'
import { parseSaved } from './persist'
import { reg } from '../test/fixtures'
import { useStore } from './store'

// Characterisation tests for the learning / interview / records actions of the store. They pin behaviour so the
// actions can move out of store.ts without changing it.

const st = () => useStore.getState()

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(1_000_000)
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
})
afterEach(() => vi.useRealTimers())

const def = { id: 's1', title: 'T', difficulty: 'Easy', summary: '', story: '', speed: 2, durationSec: 10, warmupSec: 0, goals: {}, hintCount: 2, diagram: { nodes: [], edges: [], notes: [{ id: 't1', x: 1, y: 2, text: 'hi' }] } } as unknown as Scenario
const pred = { id: 'p1', question: 'q', options: ['a', 'b'] }
const ans = (correct: boolean) => ({ choice: 0, correct, answer: 1, explain: '' })
const score = (pass: boolean, stars = 2): Score => ({ pass, stars, checks: [] }) as unknown as Score

describe('scenario entry and exit', () => {
  it('opens a fresh scenario, keeps the sandbox, and restores it on exit', () => {
    st().addNote(5, 5)
    const sandbox = st().diagram
    st().openScenario(def)
    expect(st().sandbox).toBe(sandbox)
    expect(st().diagram.notes).toHaveLength(1)
    expect(st().diagram).not.toBe(def.diagram)
    expect(st().scenario).toEqual({ def, hintLevel: 0, hints: [], debrief: null, deeper: null, answer: null, result: null, predictions: {}, solved: false, compare: null, weakPoints: [] })
    expect(st().sim).toMatchObject({ status: 'stopped', speed: 2 })
    expect(st().ui).toMatchObject({ picker: false, welcome: false, result: false })
    expect(st().past).toEqual([])
    st().openScenario({ ...def, id: 's2' }) // switching keeps the original sandbox
    expect(st().sandbox).toBe(sandbox)
    st().exitScenario()
    expect(st().scenario).toBeNull()
    expect(st().sandbox).toBeNull()
    expect(st().diagram).toBe(sandbox)
    expect(st().sim.status).toBe('stopped')
    expect(st().ui.welcome).toBe(true) // the sandbox has a note but no components
  })
  it('exits to an empty welcome sandbox when there was none', () => {
    st().openScenario(def)
    st().exitScenario()
    expect(st().diagram.nodes).toEqual([])
    expect(st().ui.welcome).toBe(true)
  })
})

describe('interview session', () => {
  it('starts, switches stage, pauses, resumes and ends', () => {
    st().startInterview(45)
    const s0 = st().interview!.session!
    expect(s0).toMatchObject({ totalMs: 45 * 60_000, startedAt: 1_000_000, pausedAt: null, stage: 'requirements' })
    expect(st().ui.welcome).toBe(false)
    vi.setSystemTime(1_060_000)
    st().interviewStage('design')
    expect(st().interview!.session!.stage).toBe('design')
    expect(st().interview!.session!.switches.at(-1)).toEqual({ stage: 'design', at: 1_060_000 })
    vi.setSystemTime(1_100_000)
    st().interviewPause(true)
    expect(st().interview!.session!.pausedAt).toBe(1_100_000)
    vi.setSystemTime(1_130_000)
    st().interviewPause(false)
    expect(st().interview!.session!).toMatchObject({ pausedAt: null, pauses: [{ from: 1_100_000, to: 1_130_000 }] })
    const pads = st().interview!
    st().endInterview()
    expect(st().interview).toEqual({ ...pads, session: null })
  })
  it('clamps the length to 1..360 minutes', () => {
    st().startInterview(0)
    expect(st().interview!.session!.totalMs).toBe(60_000)
    st().startInterview(9999)
    expect(st().interview!.session!.totalMs).toBe(360 * 60_000)
  })
  it('does nothing without a session', () => {
    st().interviewStage('api')
    st().interviewPause(true)
    st().endInterview()
    expect(st().interview).toBeNull()
    st().setInterview(null)
    expect(st().interview).toBeNull()
  })
})

describe('hints, answer, predictions and records', () => {
  it('records hints up to the hint count', () => {
    st().addHint('x') // no scenario: ignored
    expect(st().records).toEqual({})
    st().openScenario(def)
    st().addHint('h1')
    st().addHint('h2')
    st().addHint('h3') // over the count
    expect(st().scenario).toMatchObject({ hints: ['h1', 'h2'], hintLevel: 2 })
    expect(st().records.s1.hints).toBe(2)
  })
  it('records the answer and ignores it outside a scenario', () => {
    st().showAnswer('a')
    expect(st().records).toEqual({})
    st().openScenario(def)
    st().showAnswer('Run 5')
    expect(st().scenario!.answer).toBe('Run 5')
    expect(st().records.s1.answerUsed).toBe(true)
  })
  it('stores a prediction once, and only wrong ones count in the record', () => {
    st().recordPrediction(pred, ans(true)) // no scenario
    expect(st().scenario).toBeNull()
    st().openScenario(def)
    st().recordPrediction(pred, ans(true))
    expect(st().scenario!.predictions.p1).toEqual(ans(true))
    expect(st().records.s1).toBeUndefined()
    st().recordPrediction(pred, ans(false)) // already answered
    expect(st().scenario!.predictions.p1.correct).toBe(true)
    st().recordPrediction({ ...pred, id: 'p2' }, ans(false))
    expect(st().records.s1.wrong).toBe(1)
  })
  it('setResult shows the result, keeps the best stars and counts runs', () => {
    st().setResult(score(true)) // no scenario
    expect(st().progress).toEqual({})
    st().openScenario(def)
    st().setResult(score(true, 3))
    expect(st().scenario!.result).toEqual(score(true, 3))
    expect(st().ui.result).toBe(true)
    expect(st().progress.s1).toBe(3)
    st().setResult(score(false, 1))
    expect(st().progress.s1).toBe(3)
    expect(st().records.s1).toMatchObject({ runs: 1, passed: true }) // runs stop counting once passed
  })
  it('recordStars never lowers stars; importProgress replaces progress and records', () => {
    st().recordStars('a', 2)
    const p = st().progress
    st().recordStars('a', 1)
    expect(st().progress).toBe(p)
    st().recordStars('a', 3)
    expect(st().progress.a).toBe(3)
    const rec = { wrong: 1, hints: 0, runs: 0, passed: false, earlyPass: false, answerUsed: false }
    st().importProgress({ z: 1 }, { z: rec })
    expect(st().progress).toEqual({ z: 1 })
    expect(st().records).toEqual({ z: rec })
  })
  it('debrief only lands inside a scenario', () => {
    st().setDebrief('d', 'more')
    expect(st().scenario).toBeNull()
    st().openScenario(def)
    st().setDebrief('d', 'more')
    expect(st().scenario).toMatchObject({ debrief: 'd', deeper: 'more' })
  })
  it('level is changed by setLevel', () => {
    st().setLevel('sde2')
    expect(st().level).toBe('sde2')
  })
})

describe('persistence hydration round trip', () => {
  it('saved progress, records and interview come back through parseSaved and hydrate', () => {
    st().startInterview(30)
    st().openScenario(def)
    st().addHint('h')
    st().setResult(score(true, 3))
    const s = st()
    const blob = JSON.stringify({ version: 2, interview: s.interview, sandbox: s.sandbox, theme: 'dark', progress: s.progress, records: s.records, viewport: s.viewport })
    const want = { progress: s.progress, records: s.records, interview: s.interview }
    useStore.setState(useStore.getInitialState(), true)
    st().setReg(reg)
    st().hydrate(parseSaved(blob, reg))
    expect({ progress: st().progress, records: st().records, interview: st().interview }).toEqual(want)
    expect(st().theme).toBe('dark')
    expect(st().ui.welcome).toBe(true) // empty sandbox
  })
  it('an empty save leaves learning state untouched', () => {
    st().hydrate(parseSaved(null, reg))
    expect(st()).toMatchObject({ progress: {}, records: {}, interview: null })
  })
})
