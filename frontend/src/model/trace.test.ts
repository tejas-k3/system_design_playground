import { describe, expect, it } from 'vitest'
import type { Span, Trace } from '../api/types'
import { diagnose, layout, selfTimes } from './trace'

const label = (id: string) => ({ queueFull: 'Queue full (503)', timeout: 'Timeouts', connRefused: 'DB connections refused' })[id] ?? id
const span = (p: Partial<Span> & { node: string; depth: number; at: number; end: number }): Span => ({
  label: p.node, kind: 'appServer', attempt: 1, start: p.at, ok: true, ...p,
})
const trace = (p: Partial<Trace> & { spans: Span[] }): Trace => ({ id: 1, client: 'Users', write: false, key: 7, ok: true, latencyMs: 100, attempts: 1, ...p })

describe('selfTimes', () => {
  it('subtracts the time spent in direct dependencies only', () => {
    const spans = [
      span({ node: 'lb', depth: 0, at: 0, end: 100 }),
      span({ node: 'app', depth: 1, at: 5, end: 95 }),
      span({ node: 'db', depth: 2, at: 10, end: 70 }),
    ]
    expect(selfTimes(spans)).toEqual([10, 30, 60])
  })

  it('does not let one attempt leak into the next', () => {
    const spans = [span({ node: 'a', depth: 0, at: 0, end: 50 }), span({ node: 'a', depth: 0, at: 60, end: 80, attempt: 2 })]
    expect(selfTimes(spans)).toEqual([50, 20])
  })
})

describe('diagnose', () => {
  it('names the node and reason where a failure started', () => {
    const t = trace({
      ok: false, reason: 'queueFull', attempts: 1, latencyMs: 60,
      spans: [
        span({ node: 'lb', label: 'Load Balancer', depth: 0, at: 0, end: 60, ok: false, reason: 'queueFull' }),
        span({ node: 'app', label: 'App Server 2', depth: 1, at: 5, start: 5, end: 55, ok: false, reason: 'queueFull' }),
      ],
    })
    const d = diagnose(t, label)
    expect(d.headline).toBe('Failed at App Server 2: Queue full (503)')
    expect(d.culprit).toBe('app')
  })

  it('reports queueing before the rejection and retries', () => {
    const t = trace({
      ok: false, reason: 'connRefused', attempts: 3,
      spans: [span({ node: 'db', label: 'Database', depth: 0, at: 0, start: 40, end: 41, ok: false, reason: 'connRefused' })],
    })
    const d = diagnose(t, label)
    expect(d.notes.join(' ')).toContain('waited 40 ms')
    expect(d.notes.join(' ')).toContain('3 attempts')
  })

  it('explains a timeout by the slowest place and counts work that outlived the user', () => {
    const t = trace({
      ok: false, reason: 'timeout', latencyMs: 500, attempts: 1,
      spans: [
        span({ node: 'app', label: 'App Server', depth: 0, at: 0, end: 500, ok: false, open: true }),
        span({ node: 'db', label: 'Database', depth: 1, at: 20, start: 380, end: 500, kind: 'database', open: true }),
        span({ node: 'db', label: 'Database', depth: 1, at: 520, end: 700, late: true }),
      ],
    })
    const d = diagnose(t, label)
    expect(d.headline).toBe('Gave up after 500 ms (timeout)')
    expect(d.notes.join(' ')).toContain('kept working after the user gave up')
  })

  it('says where a successful request spent its time and mentions queueing', () => {
    const t = trace({
      latencyMs: 400,
      spans: [
        span({ node: 'app', label: 'App Server', depth: 0, at: 0, start: 250, end: 380 }),
      ],
    })
    const d = diagnose(t, label)
    expect(d.headline).toBe('Succeeded in 400 ms')
    expect(d.notes[0]).toContain('App Server')
    expect(d.notes[0]).toContain('waiting in its queue')
  })

  it('handles a failure that left no spans', () => {
    expect(diagnose(trace({ ok: false, reason: 'noRoute', spans: [] }), label).headline).toBe('Failed: noRoute')
  })
})

describe('layout', () => {
  it('positions spans on a shared axis and labels retries', () => {
    const t = trace({
      attempts: 2, latencyMs: 200,
      spans: [
        span({ node: 'a', depth: 0, at: 0, start: 20, end: 100 }),
        span({ node: 'a', depth: 0, at: 100, end: 200, attempt: 2 }),
      ],
    })
    const { total, rows } = layout(t)
    expect(total).toBe(200)
    expect(rows[0]).toMatchObject({ left: 0, wait: 10, width: 40 })
    expect(rows[0].attempt).toBe(1)
    expect(rows[1].attempt).toBe(2)
    expect(rows[1].left).toBe(50)
  })

  it('extends the axis to work that finished after the request did', () => {
    const t = trace({ latencyMs: 100, spans: [span({ node: 'db', depth: 0, at: 0, end: 400, late: true })] })
    expect(layout(t).total).toBe(400)
  })

  it('keeps instantaneous spans visible', () => {
    const { rows } = layout(trace({ latencyMs: 1000, spans: [span({ node: 'x', depth: 0, at: 10, end: 10 })] }))
    expect(rows[0].width).toBeGreaterThan(0)
  })
})
