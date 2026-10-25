import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { CHIP_H, chipSummary, collapseView, expandFor, groupDisplayRect, isHidden, setCollapsed, visibleIds, visibleNodes } from './collapse'

const rect = (n: { x: number; y: number }) => ({ x: n.x, y: n.y, w: 100, h: 50 })
const node = (id: string, x: number, y: number, kind = 'appServer') => ({ id, kind, x, y, label: id, params: {} })

// a region (0..800 x 0..400) with two zones: zone a (20..380) holds a1 and a2, zone b (400..780) holds b1; plus an outside client and db
const base = (): Diagram => ({
  nodes: [node('c', -300, 100, 'client'), node('a1', 40, 80), node('a2', 40, 200), node('b1', 440, 100), node('db', 1000, 100, 'database')],
  edges: [
    { id: 'e1', from: 'c', to: 'a1', latencyMs: 1 },
    { id: 'e2', from: 'c', to: 'a2', latencyMs: 1 },
    { id: 'e3', from: 'a1', to: 'a2', latencyMs: 1 },
    { id: 'e4', from: 'a1', to: 'b1', latencyMs: 1 },
    { id: 'e5', from: 'b1', to: 'db', latencyMs: 1 },
    { id: 'e6', from: 'a2', to: 'db', latencyMs: 1 },
  ],
  notes: [{ id: 'n1', x: 50, y: 300, text: 'inside a' }],
  shapes: [{ id: 's1', kind: 'ellipse', label: '', x: 50, y: 60, w: 120, h: 80 }, { id: 's2', kind: 'ellipse', label: '', x: 1000, y: 300, w: 100, h: 80 }],
  groups: [
    { id: 'r', kind: 'region', label: 'us', x: 0, y: 0, w: 800, h: 400 },
    { id: 'za', kind: 'az', label: 'a', x: 20, y: 20, w: 360, h: 360 },
    { id: 'zb', kind: 'az', label: 'b', x: 400, y: 20, w: 380, h: 360 },
  ],
})
const collapsed = (d: Diagram, ...ids: string[]): Diagram => ({ ...d, groups: d.groups!.map((g) => (ids.includes(g.id) ? { ...g, collapsed: true } : g)) })

describe('collapseView', () => {
  it('hides nothing when no box is collapsed', () => {
    const v = collapseView(base(), rect)
    expect(v.any).toBe(false)
    expect(visibleNodes(base(), v).map((n) => n.id)).toEqual(['c', 'a1', 'a2', 'b1', 'db'])
  })

  it('a collapsed zone hides its components, notes and shapes but not the others', () => {
    const d = collapsed(base(), 'za')
    const v = collapseView(d, rect)
    expect(visibleNodes(d, v).map((n) => n.id)).toEqual(['c', 'b1', 'db'])
    expect(isHidden(v, 'a1')).toBe(true)
    expect(isHidden(v, 'n1')).toBe(true)
    expect(isHidden(v, 's1')).toBe(true)
    expect(isHidden(v, 's2')).toBe(false)
    expect(isHidden(v, 'b1')).toBe(false)
    expect(isHidden(v, 'za')).toBe(false) // the box itself is the chip
    expect(isHidden(v, 'r')).toBe(false)
  })

  it('a collapsed region hides its zones and what they hold; only the region is drawn', () => {
    const d = collapsed(base(), 'r', 'za')
    const v = collapseView(d, rect)
    expect(visibleNodes(d, v).map((n) => n.id)).toEqual(['c', 'db'])
    expect(isHidden(v, 'za')).toBe(true)
    expect(isHidden(v, 'zb')).toBe(true)
    expect([...v.chips.keys()]).toEqual(['r'])
    expect(v.owner.get('a1')).toBe('r')
  })

  it('the chip is compact, sits at the box corner and never grows past the box', () => {
    const d = collapsed(base(), 'za')
    const g = d.groups!.find((x) => x.id === 'za')!
    const chip = groupDisplayRect(collapseView(d, rect), g)
    expect(chip).toMatchObject({ x: 20, y: 20, h: CHIP_H })
    expect(chip.w).toBeLessThanOrEqual(g.w)
    expect(groupDisplayRect(collapseView(base(), rect), g)).toEqual({ x: 20, y: 20, w: 360, h: 360 })
  })
})

describe('edge rerouting', () => {
  it('connections between two hidden members of one box are not drawn', () => {
    const d = collapsed(base(), 'za')
    expect(collapseView(d, rect).routes.get('e3')).toBeNull()
  })

  it('one stub per neighbour: two edges from the same visible node to hidden members share a single line', () => {
    const d = collapsed(base(), 'za')
    const v = collapseView(d, rect)
    const e1 = v.routes.get('e1')!
    const e2 = v.routes.get('e2')!
    expect(e1.draw).toBe(true)
    expect(e2.draw).toBe(false)
    expect(e2.a).toEqual(e1.a)
    expect(e2.b).toEqual(e1.b)
    expect(e1.members).toEqual(['e1', 'e2'])
    expect(e1.b).toEqual(groupDisplayRect(v, d.groups![1])) // ends at the chip, not at the hidden member
    expect(v.routes.get('e4')!.draw).toBe(true) // a1 -> b1: the box is the source end
  })

  it('opposite directions are separate stubs', () => {
    const d = collapsed(base(), 'za')
    d.edges.push({ id: 'back', from: 'a1', to: 'c', latencyMs: 1 })
    const v = collapseView(d, rect)
    expect(v.routes.get('back')!.draw).toBe(true)
    expect(v.routes.get('e1')!.draw).toBe(true)
  })

  it('edges between visible components are untouched', () => {
    const d = collapsed(base(), 'za')
    const v = collapseView(d, rect)
    expect(v.routes.get('e5')).toMatchObject({ a: rect(d.nodes[3]), b: rect(d.nodes[4]), draw: true })
  })

  it('with the region collapsed, edges to the outside end at the region chip', () => {
    const d = collapsed(base(), 'r', 'za')
    const v = collapseView(d, rect)
    expect(v.routes.get('e4')).toBeNull() // inside the region
    expect(v.routes.get('e1')!.b).toEqual(v.chips.get('r'))
    expect(v.routes.get('e5')!.a).toEqual(v.chips.get('r'))
    expect(v.routes.get('e5')!.members.sort()).toEqual(['e5', 'e6'])
  })
})

describe('chipSummary', () => {
  const stats = { a1: { util: 0.87, arr: 10, err: 0 }, a2: { util: 0.3, arr: 10, err: 0, down: true }, b1: { util: 0.99, arr: 10, err: 0 }, c: { util: 1, arr: 1, err: 0 } }
  it('counts the components and reports the busiest member, so a collapsed box cannot hide a problem', () => {
    const d = collapsed(base(), 'za')
    const s = chipSummary(collapseView(d, rect), d, 'za', stats)
    expect(s.count).toBe(2)
    expect(s.busiest).toBe(87)
    expect(s.down).toBe(1)
  })
  it('counts nested components under a collapsed region, and skips components that have no utilisation bar', () => {
    const d = collapsed(base(), 'r')
    const s = chipSummary(collapseView(d, rect), d, 'r', stats)
    expect(s.count).toBe(3)
    expect(s.busiest).toBe(99)
  })
  it('has no busiest figure before there are stats', () => {
    const d = collapsed(base(), 'za')
    expect(chipSummary(collapseView(d, rect), d, 'za', {}).busiest).toBeNull()
  })
})

describe('changing the flag', () => {
  it('setCollapsed only touches the flag of that box', () => {
    const d = base()
    const out = setCollapsed(d, 'za', true)
    expect(out.groups!.find((g) => g.id === 'za')!.collapsed).toBe(true)
    expect(out.nodes).toBe(d.nodes)
    expect(setCollapsed(out, 'za', false).groups!.find((g) => g.id === 'za')!.collapsed).toBeUndefined()
    expect(setCollapsed(d, 'nope', true)).toBe(d)
  })
  it('expandFor opens every collapsed box that hides the given item', () => {
    const d = collapsed(base(), 'r', 'za')
    const out = expandFor(d, ['a1'], rect)
    expect(out.groups!.filter((g) => g.collapsed).map((g) => g.id)).toEqual([]) // za holds a1 and r holds za
    const only = expandFor(collapsed(base(), 'za', 'zb'), ['b1'], rect)
    expect(only.groups!.filter((g) => g.collapsed).map((g) => g.id)).toEqual(['za'])
    const same = collapsed(base(), 'za')
    expect(expandFor(same, ['db'], rect)).toBe(same)
  })
  it('visibleIds drops what is hidden, including connections that are no longer drawn', () => {
    const d = collapsed(base(), 'za')
    const v = collapseView(d, rect)
    expect(visibleIds(d, v, ['a1', 'b1', 'e3', 'e2', 'e5', 'za', 'n1'])).toEqual(['b1', 'e5', 'za'])
  })
})
