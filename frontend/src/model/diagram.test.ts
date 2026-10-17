import { describe, expect, it } from 'vitest'
import { Reg, defaultLatency, paramVisible, sanitizeDiagram, uniqueLabel } from './diagram'
import type { Registry } from '../api/types'

const registry: Registry = {
  latency: { min: 1, max: 200 },
  nodes: [
    {
      kind: 'client', name: 'Users', description: '', lesson: '', accent: '#000', width: 160, height: 72,
      canConnectTo: ['appServer'],
      params: [{ key: 'rps', label: 'rps', group: 'g', type: 'number', default: 50, min: 1, max: 5000 }],
    },
    {
      kind: 'appServer', name: 'App Server', description: '', lesson: '', accent: '#000', width: 160, height: 72,
      canConnectTo: ['database'],
      params: [
        { key: 'threads', label: 't', group: 'g', type: 'number', default: 8, min: 1, max: 64 },
        { key: 'order', label: 'o', group: 'g', type: 'select', default: 'fifo', options: [{ value: 'fifo', label: '' }, { value: 'lifo', label: '' }] },
        { key: 'dbPool', label: 'p', group: 'g', type: 'number', default: 20, min: 1, max: 200, needsEdgeTo: ['database'] },
        { key: 'cb', label: 'c', group: 'g', type: 'boolean', default: false },
      ],
    },
    { kind: 'database', name: 'Database', description: '', lesson: '', accent: '#000', width: 160, height: 72, canConnectTo: null, params: [] },
  ],
}
const reg = new Reg(registry)

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
  it('uses the same default latencies as the server', () => {
    expect(defaultLatency('client', 'cdn')).toBe(10)
    expect(defaultLatency('client', 'appServer')).toBe(40)
    expect(defaultLatency('cdn', 'appServer')).toBe(40)
    expect(defaultLatency('appServer', 'externalApi')).toBe(20)
    expect(defaultLatency('appServer', 'database')).toBe(5)
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
