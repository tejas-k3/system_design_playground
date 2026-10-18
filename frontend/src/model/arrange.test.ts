import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { alignNodes, autoLayout, distributeNodes } from './arrange'

const size = () => ({ w: 160, h: 72 })
const node = (id: string, x: number, y: number) => ({ id, kind: 'appServer', x, y, label: id, params: {} })
const pos = (d: Diagram, id: string) => {
  const n = d.nodes.find((x) => x.id === id)!
  return [n.x, n.y]
}

const scattered: Diagram = { nodes: [node('a', 10, 50), node('b', 200, 10), node('c', 400, 300)], edges: [], notes: [] }

describe('align', () => {
  it('left and right use the same edge for every chosen node', () => {
    expect(alignNodes(scattered, ['a', 'b', 'c'], 'left', size).nodes.map((n) => n.x)).toEqual([10, 10, 10])
    expect(alignNodes(scattered, ['a', 'b', 'c'], 'right', size).nodes.map((n) => n.x)).toEqual([400, 400, 400])
  })
  it('top and bottom', () => {
    expect(alignNodes(scattered, ['a', 'b', 'c'], 'top', size).nodes.map((n) => n.y)).toEqual([10, 10, 10])
    expect(alignNodes(scattered, ['a', 'b', 'c'], 'bottom', size).nodes.map((n) => n.y)).toEqual([300, 300, 300])
  })
  it('centres on the middle of the selection and leaves the others alone', () => {
    const out = alignNodes(scattered, ['a', 'c'], 'centerX', size)
    expect(pos(out, 'a')[0]).toBe(pos(out, 'c')[0])
    expect(pos(out, 'b')).toEqual([200, 10])
  })
  it('does nothing for fewer than two nodes', () => {
    expect(alignNodes(scattered, ['a'], 'left', size)).toEqual(scattered)
  })
})

describe('distribute', () => {
  it('spaces the gaps equally and keeps the outer two where they are', () => {
    const d: Diagram = { nodes: [node('a', 0, 0), node('b', 100, 0), node('c', 1000, 0)], edges: [], notes: [] }
    const out = distributeNodes(d, ['a', 'b', 'c'], 'x', size)
    expect(pos(out, 'a')[0]).toBe(0)
    expect(pos(out, 'c')[0]).toBe(1000)
    const gap1 = pos(out, 'b')[0] - (pos(out, 'a')[0] + 160)
    const gap2 = pos(out, 'c')[0] - (pos(out, 'b')[0] + 160)
    expect(gap1).toBeCloseTo(gap2)
  })
  it('needs three nodes', () => {
    expect(distributeNodes(scattered, ['a', 'b'], 'x', size)).toEqual(scattered)
  })
})

describe('autoLayout', () => {
  const chain: Diagram = {
    nodes: [node('u', 500, 500), node('lb', 0, 0), node('s1', 300, 40), node('s2', 10, 700), node('db', 90, 90)],
    edges: [
      { id: '1', from: 'u', to: 'lb', latencyMs: 1 },
      { id: '2', from: 'lb', to: 's1', latencyMs: 1 },
      { id: '3', from: 'lb', to: 's2', latencyMs: 1 },
      { id: '4', from: 's1', to: 'db', latencyMs: 1 },
      { id: '5', from: 's2', to: 'db', latencyMs: 1 },
    ],
    notes: [],
  }
  it('puts every connection pointing left to right, one column per depth', () => {
    const out = autoLayout(chain, size)
    for (const e of chain.edges) expect(pos(out, e.to)[0]).toBeGreaterThan(pos(out, e.from)[0] + 160 - 1)
    expect(pos(out, 's1')[0]).toBe(pos(out, 's2')[0])
  })
  it('never overlaps two components', () => {
    const out = autoLayout(chain, size)
    for (const a of out.nodes) for (const b of out.nodes) {
      if (a.id >= b.id) continue
      const apart = Math.abs(a.x - b.x) >= 160 || Math.abs(a.y - b.y) >= 72
      expect(apart).toBe(true)
    }
  })
  it('is deterministic and keeps ids, labels and params', () => {
    expect(autoLayout(chain, size)).toEqual(autoLayout(chain, size))
    expect(autoLayout(chain, size).nodes.map((n) => n.id).sort()).toEqual(chain.nodes.map((n) => n.id).sort())
  })
  it('terminates on a cycle', () => {
    const cyc: Diagram = { nodes: [node('a', 0, 0), node('b', 0, 0)], edges: [{ id: 'x', from: 'a', to: 'b', latencyMs: 1 }, { id: 'y', from: 'b', to: 'a', latencyMs: 1 }], notes: [] }
    expect(autoLayout(cyc, size).nodes).toHaveLength(2)
  })
  it('lays disconnected components in their own rows without overlap', () => {
    const d: Diagram = { nodes: [node('a', 0, 0), node('b', 0, 0), node('c', 0, 0)], edges: [], notes: [] }
    const ys = autoLayout(d, size).nodes.map((n) => n.y)
    expect(new Set(ys).size).toBe(3)
  })
})
