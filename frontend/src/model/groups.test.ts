import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { reg } from '../test/fixtures'
import { GROUP_KINDS, clampGroupSize, groupLabel, groupMembers, groupTitle, withMembers } from './groups'
import { sanitizeDiagram } from './diagram'

const rect = (n: { x: number; y: number }) => ({ x: n.x, y: n.y, w: 100, h: 50 })
const d: Diagram = {
  nodes: [
    { id: 'in', kind: 'appServer', x: 50, y: 50, label: 'In', params: {} },
    { id: 'edge', kind: 'appServer', x: 380, y: 50, label: 'Half out', params: {} }, // centre (430) is outside a 400-wide box
    { id: 'out', kind: 'appServer', x: 900, y: 50, label: 'Out', params: {} },
  ],
  edges: [],
  notes: [
    { id: 't1', x: 60, y: 120, text: 'inside' },
    { id: 't2', x: 700, y: 10, text: 'outside' },
  ],
  groups: [
    { id: 'region', kind: 'region', label: 'us-east', x: 0, y: 0, w: 400, h: 300 },
    { id: 'az', kind: 'az', label: 'a', x: 20, y: 20, w: 200, h: 200 },
    { id: 'far', kind: 'az', label: 'b', x: 600, y: 0, w: 200, h: 200 },
  ],
}

describe('group membership', () => {
  it('a group holds components by centre, notes by corner, and smaller groups wholly inside', () => {
    expect(groupMembers(d, 'region', rect).sort()).toEqual(['az', 'in', 't1'])
    expect(groupMembers(d, 'az', rect).sort()).toEqual(['in', 't1'])
    expect(groupMembers(d, 'far', rect)).toEqual(['t2'])
  })
  it('a smaller group never contains the bigger one', () => {
    expect(groupMembers(d, 'az', rect)).not.toContain('region')
  })
  it('unknown group has no members', () => expect(groupMembers(d, 'nope', rect)).toEqual([]))
  it('withMembers adds the contents once, keeping the rest', () => {
    expect(withMembers(d, ['region', 'out'], rect).sort()).toEqual(['az', 'in', 'out', 'region', 't1'])
  })
})

describe('group helpers', () => {
  it('labels are unique per kind', () => {
    expect(groupLabel({ ...d, groups: [] }, 'az')).toBe('Availability zone')
    expect(groupLabel({ ...d, groups: [{ ...d.groups![1], label: 'Availability zone' }] }, 'az')).toBe('Availability zone 2')
  })
  it('the title always carries the kind as text, not just colour', () => {
    expect(groupTitle(d.groups![1])).toBe('AZ: a')
    expect(groupTitle({ ...d.groups![1], label: '' })).toBe('Availability zone')
  })
  it('every kind has a distinct tag and line pattern, so kinds are not told apart by colour', () => {
    expect(new Set(GROUP_KINDS.map((k) => k.tag)).size).toBe(GROUP_KINDS.length)
    expect(new Set(GROUP_KINDS.map((k) => k.dash.join(','))).size).toBe(GROUP_KINDS.length)
  })
  it('clamps sizes', () => {
    expect(clampGroupSize(5)).toBe(60)
    expect(clampGroupSize(99999)).toBe(4000)
  })
})

describe('sanitizing groups', () => {
  it('keeps good groups, and an absent field stays absent', () => {
    const out = sanitizeDiagram(reg, { nodes: [], edges: [], notes: [], groups: d.groups })!
    expect(out.adjusted).toBe(false)
    expect(out.diagram.groups).toEqual(d.groups)
    expect(sanitizeDiagram(reg, { nodes: [], edges: [], notes: [] })!.diagram.groups).toBeUndefined()
  })
  it('drops bad kinds, duplicate ids and non-numbers; clamps sizes and long labels', () => {
    const raw = {
      nodes: [],
      edges: [],
      notes: [],
      groups: [
        { id: 'a', kind: 'az', label: 'ok', x: 0, y: 0, w: 10, h: 99999 },
        { id: 'a', kind: 'az', label: 'dup', x: 0, y: 0, w: 100, h: 100 },
        { id: 'b', kind: 'moon', label: '', x: 0, y: 0, w: 100, h: 100 },
        { id: 'c', kind: 'vpc', label: 'x', x: 'q', y: 0, w: 100, h: 100 },
        { id: 'd', kind: 'vpc', label: 'x'.repeat(200), x: 1, y: 2, w: 100, h: 100, collapsed: true },
      ],
    }
    const { diagram, adjusted } = sanitizeDiagram(reg, raw)!
    expect(adjusted).toBe(true)
    expect(diagram.groups!.map((g) => g.id)).toEqual(['a', 'd'])
    expect(diagram.groups![0]).toMatchObject({ w: 60, h: 4000 })
    expect(diagram.groups![1].label).toHaveLength(60)
    expect(diagram.groups![1].collapsed).toBe(true)
  })
  it('caps the count', () => {
    const groups = Array.from({ length: 40 }, (_, i) => ({ id: `g${i}`, kind: 'az', label: '', x: 0, y: 0, w: 100, h: 100 }))
    expect(sanitizeDiagram(reg, { nodes: [], edges: [], notes: [], groups })!.diagram.groups).toHaveLength(30)
  })
})
