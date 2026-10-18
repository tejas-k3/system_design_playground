import { describe, expect, it } from 'vitest'
import { STAGES, crossedMarks, elapsedMs, newSession, pause, remainingMs, resume, stageTimes, switchStage } from './timer'

const MIN = 60_000

describe('interview timer', () => {
  it('counts from timestamps, so a 10-minute gap with nothing running costs exactly 10 minutes', () => {
    const t0 = 1_000_000
    const s = newSession(45 * MIN, t0)
    // the page is hidden: no ticks, no callbacks; when it wakes the clock reads 10 min later
    expect(elapsedMs(s, t0 + 10 * MIN)).toBe(10 * MIN)
    expect(remainingMs(s, t0 + 10 * MIN)).toBe(35 * MIN)
  })
  it('a 5-minute run is accurate to the millisecond whatever the sampling', () => {
    const t0 = 5_000
    const s = newSession(45 * MIN, t0)
    for (const dt of [0, 1, 999, 1000, 61_234, 5 * MIN]) expect(elapsedMs(s, t0 + dt)).toBe(dt)
  })
  it('pausing stops the clock and resuming carries on', () => {
    let s = newSession(45 * MIN, 0)
    s = pause(s, 2 * MIN)
    expect(elapsedMs(s, 9 * MIN)).toBe(2 * MIN)
    s = resume(s, 9 * MIN)
    expect(elapsedMs(s, 11 * MIN)).toBe(4 * MIN)
    expect(pause(pause(s, 12 * MIN), 20 * MIN)).toEqual(pause(s, 12 * MIN)) // pausing twice changes nothing
    expect(resume(s, 13 * MIN)).toEqual(s) // resuming a running timer changes nothing
  })
  it('never reports negative remaining time', () => {
    expect(remainingMs(newSession(1 * MIN, 0), 5 * MIN)).toBe(0)
  })
  it('time per stage adds up to the elapsed time, with pauses left out', () => {
    let s = newSession(45 * MIN, 0)
    s = switchStage(s, 'estimation', 5 * MIN)
    s = pause(s, 8 * MIN)
    s = resume(s, 20 * MIN)
    s = switchStage(s, 'api', 22 * MIN)
    const per = stageTimes(s, 30 * MIN)
    expect(per.requirements).toBe(5 * MIN)
    expect(per.estimation).toBe(3 * MIN + 2 * MIN) // 5-8 then 20-22
    expect(per.api).toBe(8 * MIN)
    expect(Object.values(per).reduce((a, b) => a + b, 0)).toBe(elapsedMs(s, 30 * MIN))
  })
  it('knows the seven stages in order', () => {
    expect(STAGES.map((x) => x.id)).toEqual(['requirements', 'estimation', 'api', 'data', 'design', 'deepdive', 'tradeoffs'])
  })
})

describe('announcement marks', () => {
  const s = newSession(45 * MIN, 0) // half at 22.5 min, 10 left at 35, 5 left at 40, 1 left at 44, over at 45
  it('reports nothing before the first mark', () => {
    expect(crossedMarks(s, 0, 9 * MIN)).toEqual([])
    expect(crossedMarks(s, 9 * MIN, 21 * MIN)).toEqual([])
  })
  it('reports every mark crossed while the page slept, in order, once', () => {
    expect(crossedMarks(s, 21 * MIN, 36 * MIN)).toEqual(['half', 'tenLeft'])
    expect(crossedMarks(s, 36 * MIN, 36 * MIN + 1000)).toEqual([])
    expect(crossedMarks(s, 0, 50 * MIN)).toEqual(['half', 'tenLeft', 'fiveLeft', 'oneLeft', 'over'])
  })
  it('counts a mark reached exactly at the end of the window', () => {
    expect(crossedMarks(s, 22 * MIN, 22.5 * MIN)).toEqual(['half'])
    expect(crossedMarks(s, 22.5 * MIN, 23 * MIN)).toEqual([])
  })
  it('skips marks that do not fit a short session', () => {
    const short = newSession(8 * MIN, 0) // 10 minutes left would be before the start
    expect(crossedMarks(short, 0, 9 * MIN)).toEqual(['half', 'fiveLeft', 'oneLeft', 'over'])
  })
})
