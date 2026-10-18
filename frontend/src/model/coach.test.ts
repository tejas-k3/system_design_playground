import { describe, expect, it } from 'vitest'
import type { Diagram, EdgeStats, NodeStats, Stats } from '../api/types'
import { coach } from './coach'

const node = (id: string, kind: string, params = {}) => ({ id, kind, x: 0, y: 0, label: id, params })
const edge = (from: string, to: string) => ({ id: `${from}>${to}`, from, to, latencyMs: 1 })
const d = (nodes: Diagram['nodes'], edges: Diagram['edges']): Diagram => ({ nodes, edges, notes: [] }) as Diagram
const ns = (id: string, x: Partial<NodeStats>): NodeStats => ({ id, arr: 100, ok: 100, err: 0, util: 0.1, busy: 0, cap: 8, qLen: 0, hitRate: -1, ...x })
const stats = (nodes: NodeStats[], edges: EdgeStats[] = []): Stats =>
  ({ global: { now: 30 }, nodes: Object.fromEntries(nodes.map((n) => [n.id, n])), edges: Object.fromEntries(edges.map((e) => [e.id, e])), series: [], events: [] }) as unknown as Stats

const web = d([node('app', 'appServer'), node('db', 'database')], [edge('app', 'db')])

describe('coach', () => {
  it('says nothing before a run or when everything is calm', () => {
    expect(coach(web, undefined)).toEqual([])
    expect(coach(web, stats([ns('app', { util: 0.4 }), ns('db', {})]))).toEqual([])
  })

  it('blames the threads, not the database, when the server is saturated and the database idles', () => {
    const r = coach(web, stats([ns('app', { util: 0.97, qLen: 30, busy: 8 }), ns('db', { util: 0.1 })]))
    expect(r[0].rule).toBe('threads-bottleneck')
    expect(r[0].nodeId).toBe('app')
    expect(r[0].detail).toMatch(/threads/i)
  })

  it('names the silent dependency, not the threads, when the server waits forever on a database that is cut off', () => {
    const r = coach(web, stats([ns('app', { util: 0.97, qLen: 30 }), ns('db', { util: 0, cut: true })]))
    expect(r.find((i) => i.rule === 'threads-bottleneck')).toBeUndefined()
    const stuck = r.find((i) => i.rule === 'stuck-on-dependency')
    expect(stuck?.nodeId).toBe('app')
    expect(stuck?.detail).toMatch(/call timeout/i)
    const withTimeout = d([node('app', 'appServer', { callTimeoutMs: 500 }), node('db', 'database')], [edge('app', 'db')])
    expect(coach(withTimeout, stats([ns('app', { util: 0.97, qLen: 30 }), ns('db', { util: 0, cut: true })])).find((i) => i.rule === 'stuck-on-dependency')).toBeUndefined()
  })

  it('does not blame the threads when the database is busy too', () => {
    const r = coach(web, stats([ns('app', { util: 0.97, qLen: 30 }), ns('db', { util: 0.95 })]))
    expect(r.find((i) => i.rule === 'threads-bottleneck')).toBeUndefined()
  })

  it('reports a full queue that is turning requests away', () => {
    const r = coach(web, stats([ns('app', { util: 0.8, busy: 8, cap: 8, qLen: 50, err: 20, arr: 100 }), ns('db', { util: 0.5 })]))
    expect(r.map((i) => i.rule)).toContain('queue-full')
  })

  it('reports an open circuit breaker on the calling server', () => {
    const r = coach(web, stats([ns('app', {}), ns('db', {})], [{ id: 'app>db', rps: 0, breaker: 'open' }]))
    expect(r[0].rule).toBe('breaker-open')
    expect(r[0].nodeId).toBe('app')
  })

  it('reports a failover in progress and writes refused for want of a primary', () => {
    const r = coach(web, stats([ns('app', {}), ns('db', { down: true, phase: 'promoting', phaseLeftSec: 8 })]))
    expect(r[0].rule).toBe('failover')
    expect(r[0].detail).toMatch(/8 s/)
    const r2 = coach(web, stats([ns('app', {}), ns('db', { reasons: { noPrimary: 12 } })]))
    expect(r2.map((i) => i.rule)).toContain('failover')
  })

  it('reports an autoscaler that has hit its ceiling while still hot', () => {
    const dd = d([node('app', 'appServer', { autoscale: true, maxInstances: 4, targetUtilPct: 60 }), node('db', 'database')], [edge('app', 'db')])
    expect(coach(dd, stats([ns('app', { instances: 4, util: 0.9 }), ns('db', {})])).map((i) => i.rule)).toContain('autoscale-ceiling')
    expect(coach(dd, stats([ns('app', { instances: 3, util: 0.9 }), ns('db', {})])).map((i) => i.rule)).not.toContain('autoscale-ceiling')
  })

  it('blames a cold or small cache when its hit rate is low and the database behind it is hot', () => {
    const dd = d([node('c', 'cache'), node('db', 'database')], [edge('c', 'db')])
    expect(coach(dd, stats([ns('c', { hitRate: 0.2 }), ns('db', { util: 0.9 })])).map((i) => i.rule)).toContain('cache-ineffective')
    expect(coach(dd, stats([ns('c', { hitRate: 0.8 }), ns('db', { util: 0.9 })])).map((i) => i.rule)).not.toContain('cache-ineffective')
    expect(coach(dd, stats([ns('c', { hitRate: 0.2 }), ns('db', { util: 0.3 })])).map((i) => i.rule)).not.toContain('cache-ineffective')
  })

  it('ranks errors before warnings and caps the list', () => {
    const r = coach(web, stats([ns('app', { util: 0.97, qLen: 30 }), ns('db', { down: true })]))
    const sev = r.map((i) => i.severity)
    expect(sev).toEqual([...sev].sort((a, b) => ['error', 'warn', 'info'].indexOf(a) - ['error', 'warn', 'info'].indexOf(b)))
    expect(r.length).toBeLessThanOrEqual(5)
  })
})
