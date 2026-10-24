import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import type { Reg } from '../model/diagram'
import { hitTest } from './geometry'

const reg = { spec: () => ({ width: 160, height: 72 }) } as unknown as Reg
const measure = () => ({ w: 10, h: 10 })
const d: Diagram = {
  nodes: [{ id: 'a', kind: 'appServer', x: 100, y: 100, label: 'a', params: {} }],
  edges: [],
  notes: [],
  groups: [
    { id: 'big', kind: 'region', label: 'r', x: 0, y: 0, w: 600, h: 400 },
    { id: 'small', kind: 'az', label: 'z', x: 50, y: 50, w: 300, h: 250 },
  ],
}
const none = new Set<string>()
const hit = (x: number, y: number, sel: string[] = [], zoom = 1) => hitTest({ x, y }, d, reg, zoom, measure, none, none, new Set(sel))

describe('hit testing groups', () => {
  it('the header band grabs a group; the empty body does not, so panning and marquee still work inside a zone', () => {
    expect(hit(20, 10)).toEqual({ type: 'group', id: 'big' })
    expect(hit(300, 350)).toBeNull()
  })
  it('the border grabs a group', () => {
    expect(hit(599, 200)).toEqual({ type: 'group', id: 'big' })
  })
  it('where boxes overlap the innermost (smallest) one wins', () => {
    expect(hit(100, 60)).toEqual({ type: 'group', id: 'small' })
  })
  it('a component inside a group wins over the group header', () => {
    expect(hit(110, 110)).toEqual({ type: 'node', id: 'a' })
  })
  it('a selected group shows a resize grip at its bottom-right corner', () => {
    expect(hit(600, 400, ['big'])).toEqual({ type: 'groupResize', id: 'big' })
    expect(hit(600, 400)).toEqual({ type: 'group', id: 'big' }) // still on the border, just not a grip
  })
  it('the grip stays grabbable when zoomed out', () => {
    expect(hit(600 + 8 / 0.5, 400, ['big'], 0.5)).toEqual({ type: 'groupResize', id: 'big' })
  })
})
