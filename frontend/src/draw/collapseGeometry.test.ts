import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import type { Reg } from '../model/diagram'
import { CHIP_H, collapseView } from '../model/collapse'
import { boundsOf, edgeGeom, hitTest, nodeRect } from './geometry'

const viewOf = (dg: Diagram) => collapseView(dg, (n) => nodeRect(n, reg))

const reg = { spec: () => ({ width: 160, height: 72 }) } as unknown as Reg
const measure = () => ({ w: 10, h: 10 })
const node = (id: string, x: number, y: number) => ({ id, kind: 'appServer', x, y, label: id, params: {} })
const none = new Set<string>()

const d = (collapsed: boolean): Diagram => ({
  nodes: [node('out', 700, 40), node('in1', 40, 80), node('in2', 40, 200)],
  edges: [
    { id: 'e1', from: 'out', to: 'in1', latencyMs: 1 },
    { id: 'e2', from: 'out', to: 'in2', latencyMs: 1 },
    { id: 'e3', from: 'in1', to: 'in2', latencyMs: 1 },
  ],
  notes: [{ id: 'n', x: 60, y: 150, text: 'x' }],
  shapes: [{ id: 's', kind: 'ellipse', label: '', x: 50, y: 60, w: 100, h: 60 }],
  groups: [{ id: 'g', kind: 'az', label: 'a', x: 0, y: 0, w: 400, h: 400, ...(collapsed ? { collapsed: true } : {}) }],
})
const hit = (x: number, y: number, dg: Diagram, handles = none, sel = none, selGroups = none) => hitTest({ x, y }, dg, reg, 1, measure, handles, sel, selGroups)

describe('hit testing with a collapsed box', () => {
  it('finds a member when expanded and the box chip when collapsed', () => {
    expect(hit(100, 110, d(false))).toEqual({ type: 'node', id: 'in1' })
    expect(hit(100, 30, d(true))).toEqual({ type: 'group', id: 'g' })
  })
  it('hidden notes, annotations and handles cannot be hit', () => {
    expect(hit(65, 155, d(false))?.type).toBe('note')
    expect(hit(300, 300, d(true))).toBeNull() // inside the old rectangle but outside the chip
    expect(hit(100, 60, d(true))).toEqual({ type: 'group', id: 'g' })
    const h = hit(40 + 160 + 14, 80 + 36, d(true), new Set(['in1']))
    expect(h?.type).not.toBe('handle')
  })
  it('the whole chip grabs the box, not only a header band', () => {
    expect(hit(100, CHIP_H - 4, d(true))).toEqual({ type: 'group', id: 'g' })
    expect(hit(100, CHIP_H + 20, d(true))).toBeNull()
  })
  it('a collapsed box has no resize grip', () => {
    expect(hit(400, 400, d(true), none, none, new Set(['g']))).toBeNull()
    expect(hit(400, 400, d(false), none, none, new Set(['g']))).toEqual({ type: 'groupResize', id: 'g' })
  })
  it('a connection to a hidden member is hit along its stub to the chip; a folded duplicate is not hit separately', () => {
    const dg = d(true)
    const g = edgeGeom(dg.edges[0], new Map(dg.nodes.map((n) => [n.id, n])), reg, viewOf(dg))!
    expect(hit((g.a.x + g.b.x) / 2, (g.a.y + g.b.y) / 2, dg)).toEqual({ type: 'edge', id: 'e1' })
  })
})

describe('edge geometry with a collapsed box', () => {
  it('ends at the chip outline instead of the hidden member, and is null for an inner connection', () => {
    const dg = d(true)
    const nodes = new Map(dg.nodes.map((n) => [n.id, n]))
    const e = edgeGeom(dg.edges[0], nodes, reg, viewOf(dg))!
    expect(e.b.x).toBeGreaterThanOrEqual(260 - 1) // on the right edge of the 260 wide chip
    expect(e.b.x).toBeLessThanOrEqual(260 + 8)
    expect(edgeGeom(dg.edges[2], nodes, reg, viewOf(dg))).toBeNull()
    const open = edgeGeom(d(false).edges[0], nodes, reg, viewOf(d(false)))!
    expect(open.b.x).toBeCloseTo(200 + 6, -1)
  })
})

describe('bounds with a collapsed box', () => {
  it('cover the chip, not the hidden members', () => {
    const open = boundsOf(d(false), reg, measure)!
    expect(open).toMatchObject({ x: 0, y: 0, w: 860, h: 400 })
    const closed = boundsOf(d(true), reg, measure)!
    expect(closed.h).toBeLessThan(120)
    expect(closed.x).toBe(0)
  })
})
