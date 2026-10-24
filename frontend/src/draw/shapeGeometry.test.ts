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
  groups: [{ id: 'box', kind: 'region', label: 'r', x: 0, y: 0, w: 800, h: 600 }],
  shapes: [
    { id: 'ell', kind: 'ellipse', label: '', x: 300, y: 300, w: 200, h: 100 },
    { id: 'arr', kind: 'arrow', label: '', x: 500, y: 200, w: 100, h: 0 },
  ],
}
const none = new Set<string>()
const hit = (x: number, y: number, sel: string[] = [], zoom = 1) => hitTest({ x, y }, d, reg, zoom, measure, none, none, none, new Set(sel))

describe('hit testing shapes', () => {
  it('an ellipse is grabbed on its outline, the inside is empty so panning and marquee still work', () => {
    expect(hit(300, 350)).toEqual({ type: 'shape', id: 'ell' })
    expect(hit(400, 350)).toBeNull()
  })
  it('an arrow is grabbed along its shaft', () => {
    expect(hit(550, 203)).toEqual({ type: 'shape', id: 'arr' })
    expect(hit(550, 240)).toBeNull()
  })
  it('a shape beats the box border and header behind it; a component beats a shape', () => {
    expect(hit(300, 10)).toEqual({ type: 'group', id: 'box' })
    const over: Diagram = { ...d, shapes: [{ id: 'e2', kind: 'ellipse', label: '', x: 0, y: -50, w: 200, h: 100 }] }
    expect(hitTest({ x: 0, y: 0 }, over, reg, 1, measure, none, none, none, none)).toEqual({ type: 'shape', id: 'e2' })
    const onNode: Diagram = { ...d, shapes: [{ id: 'e3', kind: 'ellipse', label: '', x: 60, y: 90, w: 200, h: 100 }] }
    expect(hitTest({ x: 150, y: 140 }, onNode, reg, 1, measure, none, none, none, none)).toEqual({ type: 'node', id: 'a' })
  })
  it('a selected ellipse has a resize grip at its bottom-right corner', () => {
    expect(hit(500, 400, ['ell'])).toEqual({ type: 'shapeResize', id: 'ell' })
    expect(hit(500 + 8 / 0.5, 400, ['ell'], 0.5)).toEqual({ type: 'shapeResize', id: 'ell' })
  })
  it('a selected arrow has a grip on each end', () => {
    expect(hit(600, 200, ['arr'])).toEqual({ type: 'shapeEnd', id: 'arr', end: 'to' })
    expect(hit(500, 200, ['arr'])).toEqual({ type: 'shapeEnd', id: 'arr', end: 'from' })
    expect(hit(600, 200)).toEqual({ type: 'shape', id: 'arr' }) // still on the line when not selected
  })
})
