import { describe, expect, it } from 'vitest'
import type { SimEvent } from '../api/types'
import { eventGlyph, eventLabel, eventMarks } from './events'

const ev = (t: number, type: string, label = 'Database', extra: Partial<SimEvent> = {}): SimEvent => ({ t, type, nodeId: 'db', label, ...extra })

describe('eventLabel', () => {
  it('says what happened in plain words', () => {
    expect(eventLabel(ev(20, 'crash'))).toBe('Database crashed')
    expect(eventLabel(ev(30, 'restart'))).toBe('Database restarted')
    expect(eventLabel(ev(15, 'partition'))).toBe('Database cut off from the network')
    expect(eventLabel(ev(22, 'heal'))).toBe('Database reconnected')
    expect(eventLabel(ev(25, 'failoverDetected'))).toBe('Failure of Database noticed')
    expect(eventLabel(ev(31, 'compaction'))).toBe('Database is compacting: writes wait')
    expect(eventLabel(ev(30, 'gc'))).toBe('Database paused for garbage collection')
    expect(eventLabel(ev(35, 'scaleUp', 'App Server'))).toBe('App Server: another server is booting')
    expect(eventLabel(ev(45, 'serverUp', 'App Server'))).toBe('App Server: another server is running')
  })
  it('includes the writes lost when there were some', () => {
    expect(eventLabel(ev(25, 'promotion', 'Read Replica', { lost: 168 }))).toBe('Read Replica promoted to primary (168 writes lost)')
    expect(eventLabel(ev(25, 'promotion', 'Read Replica', { lost: 0 }))).toBe('Read Replica promoted to primary (no writes lost)')
    expect(eventLabel(ev(50, 'splitbrain', 'Database', { lost: 1972 }))).toBe('Database rejoined as primary: 1972 writes lost')
  })
  it('words a configuration push', () => {
    expect(eventLabel(ev(1, 'configPush'))).toBe('Database was given a new configuration')
  })
  it('falls back to the type for an event it does not know', () => {
    expect(eventLabel(ev(1, 'mystery'))).toBe('Database: mystery')
  })
})

describe('eventGlyph', () => {
  it('gives every known event a distinct one-character cue so markers do not rely on colour', () => {
    const types = ['crash', 'restart', 'slow', 'slowEnd', 'partition', 'heal', 'failoverDetected', 'promotion', 'splitbrain', 'scaleUp', 'serverUp', 'scaleDown', 'gc', 'compaction', 'configPush']
    const glyphs = types.map(eventGlyph)
    expect(new Set(glyphs).size).toBe(types.length)
    expect(glyphs.every((g) => g.length === 1)).toBe(true)
  })
})

describe('eventMarks', () => {
  const geom = { t0: 0, t1: 100, left: 40, width: 400 }
  it('places a marker at the chart x of its time', () => {
    const [m] = eventMarks([ev(25, 'crash')], geom)
    expect(m.x).toBeCloseTo(40 + 0.25 * 400)
    expect(m.glyph).toBe(eventGlyph('crash'))
    expect(m.labels).toEqual(['Database crashed'])
  })
  it('drops events outside the visible window', () => {
    expect(eventMarks([ev(-5, 'crash'), ev(120, 'crash')], geom)).toEqual([])
  })
  it('merges markers closer than the minimum gap into one that lists all of them', () => {
    const marks = eventMarks([ev(50, 'crash'), ev(50.5, 'failoverDetected'), ev(80, 'promotion')], geom)
    expect(marks).toHaveLength(2)
    expect(marks[0].labels).toEqual(['Database crashed', 'Failure of Database noticed'])
    expect(marks[0].count).toBe(2)
  })
  it('keeps order by time', () => {
    const marks = eventMarks([ev(80, 'heal'), ev(20, 'partition')], geom)
    expect(marks.map((m) => m.x)).toEqual([...marks.map((m) => m.x)].sort((a, b) => a - b))
  })
  it('is empty with a degenerate window', () => {
    expect(eventMarks([ev(5, 'crash')], { t0: 5, t1: 5, left: 0, width: 100 })).toEqual([])
  })
})
