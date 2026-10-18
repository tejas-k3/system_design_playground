import { describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import { connectableTargets, nothingToRun, openOutputs, paramVisible, sanitizeDiagram, uniqueLabel } from './diagram'



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

describe('nothingToRun', () => {
  const node = (id: string, kind: string) => ({ id, kind, x: 0, y: 0, label: id, params: {} })
  const dia = (nodes: ReturnType<typeof node>[], edges: { from: string; to: string }[]) =>
    ({ nodes, edges: edges.map((e) => ({ id: e.from + e.to, ...e, latencyMs: 5 })), notes: [] }) as unknown as Parameters<typeof nothingToRun>[0]

  it('says to add Users when the canvas has no traffic source', () => {
    expect(nothingToRun(dia([], []))).toMatch(/Add Users/)
    expect(nothingToRun(dia([node('a', 'appServer')], []))).toMatch(/Add Users/)
  })
  it('says to connect Users when nothing receives their requests', () => {
    expect(nothingToRun(dia([node('u', 'client'), node('a', 'appServer')], []))).toMatch(/Connect Users/)
  })
  it('lets a wired design run', () => {
    expect(nothingToRun(dia([node('u', 'client'), node('a', 'appServer')], [{ from: 'u', to: 'a' }]))).toBeNull()
  })
})

describe('optionLabel', () => {
  it('names the chosen option of a select setting', () => {
    expect(reg.optionLabel('appServer', 'order', 'lifo')).toBe('Newest first')
  })
  it('returns undefined for anything it cannot name', () => {
    expect(reg.optionLabel('appServer', 'order', 'nope')).toBeUndefined()
    expect(reg.optionLabel('appServer', 'threads', 8)).toBeUndefined()
    expect(reg.optionLabel('ghost', 'order', 'lifo')).toBeUndefined()
  })
})

describe('edge bandwidth in the sanitizer', () => {
  const base = { nodes: [{ id: 'u', kind: 'client', x: 0, y: 0, label: 'U', params: {} }, { id: 'a', kind: 'appServer', x: 1, y: 1, label: 'A', params: {} }], notes: [] }
  const edge = (bw: unknown) => ({ id: 'u>a', from: 'u', to: 'a', latencyMs: 40, bandwidthMbps: bw })
  it('keeps a valid limit and drops 0 and nonsense', () => {
    expect(sanitizeDiagram(reg, { ...base, edges: [edge(80)] })?.diagram.edges[0].bandwidthMbps).toBe(80)
    expect(sanitizeDiagram(reg, { ...base, edges: [edge(0)] })?.diagram.edges[0].bandwidthMbps).toBeUndefined()
    const bad = sanitizeDiagram(reg, { ...base, edges: [edge(-4)] })
    expect(bad?.diagram.edges[0].bandwidthMbps).toBeUndefined()
    expect(bad?.adjusted).toBe(true)
    expect(sanitizeDiagram(reg, { ...base, edges: [edge('fast')] })?.diagram.edges[0].bandwidthMbps).toBeUndefined()
  })
  it('clamps an absurd limit to 100,000 Mbit/s', () => {
    expect(sanitizeDiagram(reg, { ...base, edges: [edge(1e9)] })?.diagram.edges[0].bandwidthMbps).toBe(100000)
  })
})

describe('edge loss and jitter in the sanitizer', () => {
  const base = { nodes: [{ id: 'u', kind: 'client', x: 0, y: 0, label: 'U', params: {} }, { id: 'a', kind: 'appServer', x: 1, y: 1, label: 'A', params: {} }], notes: [] }
  const edge = (extra: Record<string, unknown>) => ({ id: 'u>a', from: 'u', to: 'a', latencyMs: 40, ...extra })
  it('keeps valid values and omits zeros', () => {
    const e = sanitizeDiagram(reg, { ...base, edges: [edge({ lossPct: 5, jitterMs: 30 })] })?.diagram.edges[0]
    expect(e?.lossPct).toBe(5)
    expect(e?.jitterMs).toBe(30)
    const z = sanitizeDiagram(reg, { ...base, edges: [edge({ lossPct: 0, jitterMs: 0 })] })?.diagram.edges[0]
    expect(z?.lossPct).toBeUndefined()
    expect(z?.jitterMs).toBeUndefined()
  })
  it('clamps loss to 0-100 and jitter to 1,000 ms, and drops nonsense', () => {
    const e = sanitizeDiagram(reg, { ...base, edges: [edge({ lossPct: 400, jitterMs: 1e9 })] })
    expect(e?.diagram.edges[0].lossPct).toBe(100)
    expect(e?.diagram.edges[0].jitterMs).toBe(1000)
    expect(e?.adjusted).toBe(true)
    const bad = sanitizeDiagram(reg, { ...base, edges: [edge({ lossPct: -2, jitterMs: 'x' })] })
    expect(bad?.diagram.edges[0].lossPct).toBeUndefined()
    expect(bad?.diagram.edges[0].jitterMs).toBeUndefined()
  })
})

describe('edge protocol and async in the sanitizer', () => {
  const base = { nodes: [{ id: 'u', kind: 'client', x: 0, y: 0, label: 'U', params: {} }, { id: 'a', kind: 'appServer', x: 1, y: 1, label: 'A', params: {} }], notes: [] }
  const edge = (extra: Record<string, unknown>) => ({ id: 'u>a', from: 'u', to: 'a', latencyMs: 40, ...extra })
  it('keeps a known protocol and the async flag', () => {
    const e = sanitizeDiagram(reg, { ...base, edges: [edge({ protocol: 'grpc', async: true })] })
    expect(e?.diagram.edges[0].protocol).toBe('grpc')
    expect(e?.diagram.edges[0].async).toBe(true)
    expect(e?.adjusted).toBe(false)
  })
  it('drops an unknown protocol and a false or non-boolean async, and says it adjusted', () => {
    const e = sanitizeDiagram(reg, { ...base, edges: [edge({ protocol: 'pigeon', async: 'yes' })] })
    expect(e?.diagram.edges[0].protocol).toBeUndefined()
    expect(e?.diagram.edges[0].async).toBeUndefined()
    expect(e?.adjusted).toBe(true)
    expect(sanitizeDiagram(reg, { ...base, edges: [edge({ async: false })] })?.diagram.edges[0].async).toBeUndefined()
  })
})
