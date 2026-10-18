import { describe, expect, it } from 'vitest'
import type { Diagram, Finding } from '../api/types'
import { emptyInterview } from './interview'
import { checkTargets, rubricEvidence } from './rubric'

const node = (id: string, kind: string, params = {}) => ({ id, kind, x: 0, y: 0, label: id, params })
const dia = (nodes: Diagram['nodes']): Diagram => ({ nodes, edges: [], notes: [] }) as unknown as Diagram
const f = (rule: string): Finding => ({ rule, severity: 'warn', message: '', fix: '', nodes: [] })

describe('checkTargets', () => {
  const nf = (label: string, target: number | null, unit: string) => ({ label, target, unit })
  it('reads availability and success targets in % against the success rate', () => {
    const rows = checkTargets([nf('Availability', 99.9, '%')], { success: 0.9995, p95: 120 })
    expect(rows).toEqual([{ label: 'Availability', target: '99.9 %', actual: '99.95 %', ok: true }])
    expect(checkTargets([nf('Success rate', 99, '%')], { success: 0.95, p95: 0 })[0].ok).toBe(false)
  })
  it('reads latency targets in ms against p95', () => {
    expect(checkTargets([nf('p95 latency', 300, 'ms')], { success: 1, p95: 250 })[0]).toEqual({ label: 'p95 latency', target: '300 ms', actual: '250 ms', ok: true })
    expect(checkTargets([nf('Latency', 100, 'ms')], { success: 1, p95: 250 })[0].ok).toBe(false)
  })
  it('says what it cannot test instead of guessing', () => {
    const rows = checkTargets([nf('Durability', 11, 'nines'), nf('Open question', null, ''), nf('Availability', null, '%')], { success: 1, p95: 1 })
    expect(rows.map((r) => r.ok)).toEqual([null, null, null])
    expect(rows[1].actual).toMatch(/no number/i)
    expect(rows[0].actual).toMatch(/cannot be tested/i)
  })
})

describe('rubricEvidence', () => {
  const base = { data: emptyInterview(), findings: [] as Finding[], run: null as { success: number; p95: number; costPerHour: number } | null }
  const facts = (diagram: Diagram, over: Partial<typeof base> = {}) => rubricEvidence(diagram, { ...base, ...over })
  const find = (list: ReturnType<typeof facts>, area: string, text: RegExp) => list.find((x) => x.area === area && text.test(x.text))

  it('credits more than one app server, and autoscaling, for scalability', () => {
    expect(find(facts(dia([node('a', 'appServer')])), 'scalability', /one app server/i)?.ok).toBe(false)
    expect(find(facts(dia([node('a', 'appServer'), node('b', 'appServer')])), 'scalability', /app servers/i)?.ok).toBe(true)
    expect(find(facts(dia([node('a', 'appServer', { autoscale: true })])), 'scalability', /autoscal/i)?.ok).toBe(true)
  })
  it('takes availability facts from the lint findings', () => {
    const bad = facts(dia([node('db', 'database')]), { findings: [f('spof-database'), f('no-call-timeout')] })
    expect(find(bad, 'availability', /only copy/i)?.ok).toBe(false)
    expect(find(bad, 'availability', /timeout/i)?.ok).toBe(false)
    const good = facts(dia([node('db', 'database')]))
    expect(find(good, 'availability', /lone database/i)?.ok).toBe(true)
    expect(find(good, 'availability', /timeout/i)?.ok).toBe(true)
  })
  it('takes consistency facts from the failover and cache findings', () => {
    expect(find(facts(dia([]), { findings: [f('unfenced-failover')] }), 'consistency', /fence|split/i)?.ok).toBe(false)
    expect(find(facts(dia([]), { findings: [f('cache-no-ttl-with-writes')] }), 'consistency', /cache/i)?.ok).toBe(false)
  })
  it('reports measured success, latency and cost once a run exists, and says there is none before', () => {
    const before = facts(dia([]))
    expect(before.some((x) => x.area === 'cost' && /run/i.test(x.text) && x.ok === null)).toBe(true)
    const after = facts(dia([]), { run: { success: 0.991, p95: 210, costPerHour: 1.42 } })
    expect(find(after, 'cost', /\$1\.42/)?.ok).toBeNull()
    expect(find(after, 'scalability', /99\.1 %/)).toBeDefined()
  })
  it('judges communication from what was written', () => {
    const empty = facts(dia([]))
    expect(find(empty, 'communication', /functional/i)?.ok).toBe(false)
    const d = { ...emptyInterview(), functional: ['a', 'b', 'c'], nonFunctional: [{ label: 'x', target: 1, unit: '%' }, { label: 'y', target: 2, unit: 'ms' }, { label: 'z', target: 3, unit: 'GB' }], notes: 'some' }
    const full = facts(dia([]), { data: d })
    expect(find(full, 'communication', /functional/i)?.ok).toBe(true)
    expect(find(full, 'communication', /numbers/i)?.ok).toBe(true)
  })
})
