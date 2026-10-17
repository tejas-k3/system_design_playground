import { describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import { connectableTargets, openOutputs, paramVisible, sanitizeDiagram, uniqueLabel } from './diagram'



describe('sanitizeDiagram', () => {
  it('rejects non-objects', () => {
    expect(sanitizeDiagram(reg, null)).toBeNull()
    expect(sanitizeDiagram(reg, { nodes: 'x' })).toBeNull()
  })

  it('drops bad nodes and edges, clamps params and latency', () => {
    const out = sanitizeDiagram(reg, {
      nodes: [
        { id: 'a', kind: 'appServer', x: 1, y: 2, params: { threads: 999, order: 'weird', bogus: 1, cb: 'yes' } },
        { id: 'a', kind: 'database', x: 0, y: 0 },
        { id: 'd', kind: 'database', x: 0, y: 0 },
        { id: 'z', kind: 'nope', x: 0, y: 0 },
        { id: 'n', kind: 'client', x: NaN, y: 0 },
      ],
      edges: [
        { id: 'e1', from: 'a', to: 'd', latencyMs: 9999 },
        { id: 'e2', from: 'd', to: 'a', latencyMs: 5 },
        { id: 'e3', from: 'a', to: 'd', latencyMs: 5 },
        { id: 'e4', from: 'a', to: 'a', latencyMs: 5 },
      ],
    })!
    expect(out.adjusted).toBe(true)
    expect(out.diagram.nodes.map((n) => n.id)).toEqual(['a', 'd'])
    expect(out.diagram.nodes[0].params).toEqual({ threads: 64 })
    expect(out.diagram.edges).toEqual([{ id: 'e1', from: 'a', to: 'd', latencyMs: 200 }])
    expect(out.diagram.nodes[0].label).toBe('App Server')
  })

  it('keeps a valid diagram untouched', () => {
    const d = {
      nodes: [{ id: 'u', kind: 'client', x: 0, y: 0, label: 'Users', params: { rps: 10 } }],
      edges: [],
      notes: [{ id: 'n1', x: 3, y: 4, text: 'hi' }],
    }
    const out = sanitizeDiagram(reg, d)!
    expect(out.adjusted).toBe(false)
    expect(out.diagram).toEqual(d)
  })
})

describe('helpers', () => {
  it('applies the default-latency rules the server provides, first match winning', () => {
    expect(reg.defaultLatency('client', 'cdn')).toBe(10)
    expect(reg.defaultLatency('client', 'appServer')).toBe(40)
    expect(reg.defaultLatency('cdn', 'appServer')).toBe(40)
    expect(reg.defaultLatency('appServer', 'externalApi')).toBe(20)
    expect(reg.defaultLatency('appServer', 'database')).toBe(5)
  })

  it('labels failure reasons from the registry and falls back to the id', () => {
    expect(reg.reasonLabel('queueFull')).toBe('Queue full (503)')
    expect(reg.reasonLabel('mystery')).toBe('mystery')
    expect(reg.speeds).toEqual([0.5, 1, 2])
  })

  it('lists only the components a node may still connect to', () => {
    const node = (id: string, kind: string, label: string) => ({ id, kind, x: 0, y: 0, label, params: {} })
    const d = {
      nodes: [node('u', 'client', 'Users'), node('a1', 'appServer', 'B server'), node('a2', 'appServer', 'A server'), node('db', 'database', 'DB')],
      edges: [{ id: 'e1', from: 'u', to: 'a1', latencyMs: 40 }],
      notes: [],
    }
    expect(connectableTargets(d, reg, 'u').map((n) => n.id)).toEqual(['a2']) // a1 already linked, db not allowed
    expect(connectableTargets(d, reg, 'a1').map((n) => n.id)).toEqual(['db'])
    expect(connectableTargets(d, reg, 'db')).toEqual([]) // databases connect to nothing
    expect(connectableTargets(d, reg, 'missing')).toEqual([])
  })

  it('numbers duplicate labels', () => {
    const d = { nodes: [{ label: 'Cache' }, { label: 'Cache 2' }], edges: [], notes: [] } as never
    expect(uniqueLabel(d, 'Cache')).toBe('Cache 3')
    expect(uniqueLabel(d, 'Other')).toBe('Other')
  })

  it('hides params whose edge requirement is not met', () => {
    const node = { id: 'a', kind: 'appServer', x: 0, y: 0, label: '', params: {} }
    const pool = reg.params('appServer').find((p) => p.key === 'dbPool')!
    const without = { nodes: [node], edges: [], notes: [] }
    const withEdge = { nodes: [node, { id: 'd', kind: 'database', x: 0, y: 0, label: '', params: {} }], edges: [{ id: 'e', from: 'a', to: 'd', latencyMs: 5 }], notes: [] }
    expect(paramVisible(without, node, pool, reg.defaults('appServer'))).toBe(false)
    expect(paramVisible(withEdge, node, pool, reg.defaults('appServer'))).toBe(true)
  })
})

describe('openOutputs', () => {
  const d = {
    nodes: [
      { id: 'u', kind: 'client', x: 0, y: 0, label: 'u', params: {} },
      { id: 'a', kind: 'appServer', x: 0, y: 0, label: 'a', params: {} },
      { id: 'd', kind: 'database', x: 0, y: 0, label: 'd', params: {} },
    ],
    edges: [{ id: 'u>a', from: 'u', to: 'a', latencyMs: 5 }],
    notes: [],
  } as unknown as Parameters<typeof openOutputs>[1]

  it('lists components that can send requests but do not yet send them anywhere', () => {
    expect([...openOutputs(reg, d)]).toEqual(['a'])
  })
  it('is empty once everything that can connect is connected', () => {
    const done = { ...d, edges: [...d.edges, { id: 'a>d', from: 'a', to: 'd', latencyMs: 5 }] }
    expect(openOutputs(reg, done).size).toBe(0)
  })
})
