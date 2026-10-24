import { describe, expect, it } from 'vitest'
import type { Diagram, Group } from '../api/types'
import type { Rect } from '../draw/geometry'
import { tidyDiagram } from './tidy'

const size = () => ({ w: 160, h: 72 })
const node = (id: string, x: number, y: number) => ({ id, kind: 'appServer', x, y, label: id, params: {} })
const box = (id: string, x: number, y: number, w: number, h: number): Group => ({ id, kind: 'az', label: id, x, y, w, h })
const rectOf = (d: Diagram, id: string): Rect => {
  const n = d.nodes.find((x) => x.id === id)!
  return { x: n.x, y: n.y, w: 160, h: 72 }
}
const gRect = (d: Diagram, id: string): Rect => {
  const g = d.groups!.find((x) => x.id === id)!
  return { x: g.x, y: g.y, w: g.w, h: g.h }
}
const inside = (o: Rect, r: Rect) => r.x >= o.x && r.y >= o.y && r.x + r.w <= o.x + o.w && r.y + r.h <= o.y + o.h
const overlap = (a: Rect, b: Rect) => a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h

// two zones, each holding a web server and a db, deliberately scattered and interleaved
const two: Diagram = {
  nodes: [node('lb', 0, 100), node('w1', 300, 0), node('w2', 300, 400), node('d1', 600, 0), node('d2', 600, 400)],
  edges: [
    { id: 'e1', from: 'lb', to: 'w1', latencyMs: 1 },
    { id: 'e2', from: 'lb', to: 'w2', latencyMs: 1 },
    { id: 'e3', from: 'w1', to: 'd1', latencyMs: 1 },
    { id: 'e4', from: 'w2', to: 'd2', latencyMs: 1 },
  ],
  notes: [],
  groups: [box('za', 280, -30, 520, 140), box('zb', 280, 370, 520, 140)],
}

describe('tidyDiagram with boxes', () => {
  it('without boxes it is the plain auto layout', () => {
    const d: Diagram = { ...two, groups: undefined }
    const out = tidyDiagram(d, size)
    expect(out.groups).toBeUndefined()
    expect(out.nodes.length).toBe(5)
  })

  it('each box wraps its members after the tidy', () => {
    const out = tidyDiagram(two, size)
    for (const id of ['w1', 'd1']) expect(inside(gRect(out, 'za'), rectOf(out, id))).toBe(true)
    for (const id of ['w2', 'd2']) expect(inside(gRect(out, 'zb'), rectOf(out, id))).toBe(true)
  })

  it('a box never covers a component that is not its member, and boxes do not overlap', () => {
    const out = tidyDiagram(two, size)
    for (const id of ['lb', 'w2', 'd2']) expect(overlap(gRect(out, 'za'), rectOf(out, id))).toBe(false)
    for (const id of ['lb', 'w1', 'd1']) expect(overlap(gRect(out, 'zb'), rectOf(out, id))).toBe(false)
    expect(overlap(gRect(out, 'za'), gRect(out, 'zb'))).toBe(false)
  })

  it('leaves the header room above the members', () => {
    const out = tidyDiagram(two, size)
    expect(rectOf(out, 'w1').y - gRect(out, 'za').y).toBeGreaterThanOrEqual(30)
  })

  it('nested boxes: the inner box sits inside the outer one with its members', () => {
    const d: Diagram = {
      nodes: [node('a', 50, 50), node('b', 500, 500), node('c', 900, 0)],
      edges: [{ id: 'e', from: 'a', to: 'b', latencyMs: 1 }, { id: 'f', from: 'b', to: 'c', latencyMs: 1 }],
      notes: [],
      groups: [box('region', 0, 0, 700, 700), box('zone', 20, 20, 300, 200)],
    }
    const out = tidyDiagram(d, size)
    expect(inside(gRect(out, 'zone'), rectOf(out, 'a'))).toBe(true)
    expect(inside(gRect(out, 'region'), gRect(out, 'zone'))).toBe(true)
    expect(inside(gRect(out, 'region'), rectOf(out, 'b'))).toBe(true)
    expect(overlap(gRect(out, 'region'), rectOf(out, 'c'))).toBe(false)
  })

  it('a note inside a box travels with it', () => {
    const d: Diagram = { ...two, notes: [{ id: 't', x: 300, y: -10, text: 'hi' }] }
    const out = tidyDiagram(d, size)
    const g = gRect(out, 'za')
    const n = out.notes[0]
    expect(n.x >= g.x && n.x <= g.x + g.w && n.y >= g.y && n.y <= g.y + g.h).toBe(true)
  })

  it('an empty box keeps its size and does not sit on a component', () => {
    const d: Diagram = { ...two, groups: [...two.groups!, box('empty', 2000, 2000, 200, 100)] }
    const out = tidyDiagram(d, size)
    expect(gRect(out, 'empty').w).toBe(200)
    for (const n of out.nodes) expect(overlap(gRect(out, 'empty'), rectOf(out, n.id))).toBe(false)
  })

  it('is deterministic', () => {
    expect(tidyDiagram(two, size)).toEqual(tidyDiagram(two, size))
  })
})
