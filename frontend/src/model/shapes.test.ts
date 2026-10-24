import { describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import { sanitizeDiagram } from './diagram'
import { addUserTemplate } from './userTemplates'
import { arrowEnds, clampShape, newShape, onShape, shapeRect, shapeTitle } from './shapes'
import { copySelection, pasteClip } from './clipboard'

const san = (raw: unknown) => sanitizeDiagram(reg, raw)!

describe('shape geometry', () => {
  it('clamps an ellipse to a positive size and an arrow to a signed extent that is never zero', () => {
    expect(clampShape('ellipse', 2, 99999)).toEqual({ w: 20, h: 4000 })
    expect(clampShape('arrow', -99999, 5)).toEqual({ w: -4000, h: 5 })
    expect(clampShape('arrow', 0, 0).w).toBeGreaterThan(0)
  })
  it('the rect of an arrow that points up-left is normalised', () => {
    expect(shapeRect({ ...newShape('s', 'arrow', 100, 100), w: -40, h: -10 })).toEqual({ x: 60, y: 90, w: 40, h: 10 })
  })
  it('an ellipse is grabbed on its outline, not in its middle', () => {
    const e = { ...newShape('s', 'ellipse', 0, 0), w: 200, h: 100 }
    expect(onShape({ x: 0, y: 50 }, e, 4)).toBe(true) // leftmost point
    expect(onShape({ x: 100, y: 0 }, e, 4)).toBe(true) // top
    expect(onShape({ x: 100, y: 50 }, e, 4)).toBe(false) // centre
    expect(onShape({ x: 0, y: 0 }, e, 4)).toBe(false) // corner of the bounding box lies outside the curve
  })
  it('an arrow is grabbed along its shaft', () => {
    const a = { ...newShape('s', 'arrow', 0, 0), w: 100, h: 0 }
    expect(arrowEnds(a).b).toEqual({ x: 100, y: 0 })
    expect(onShape({ x: 50, y: 3 }, a, 4)).toBe(true)
    expect(onShape({ x: 50, y: 20 }, a, 4)).toBe(false)
  })
  it('titles are text', () => {
    expect(shapeTitle({ ...newShape('s', 'arrow', 0, 0), label: 'reads' })).toBe('Arrow: reads')
    expect(shapeTitle(newShape('s', 'ellipse', 0, 0))).toBe('Ellipse')
  })
})

describe('shapes in a saved design', () => {
  const raw = {
    nodes: [],
    edges: [],
    notes: [],
    shapes: [
      { id: 's1', kind: 'ellipse', label: 'hot', x: 1, y: 2, w: 200, h: 100 },
      { id: 's2', kind: 'arrow', label: '', x: 0, y: 0, w: -50, h: 20 },
      { id: 's3', kind: 'star', x: 0, y: 0, w: 50, h: 50 },
      { id: 's1', kind: 'ellipse', x: 0, y: 0, w: 50, h: 50 },
      { id: 's4', kind: 'ellipse', x: 0, y: 0, w: 1, h: 99999 },
    ],
  }
  it('keeps good shapes, drops unknown kinds and duplicate ids, clamps sizes', () => {
    const { diagram, adjusted } = san(raw)
    expect(adjusted).toBe(true)
    expect(diagram.shapes!.map((s) => s.id)).toEqual(['s1', 's2', 's4'])
    expect(diagram.shapes![2]).toMatchObject({ w: 20, h: 4000 })
    expect(diagram.shapes![1]).toMatchObject({ w: -50, h: 20 })
  })
  it('an old design without shapes loads unchanged and gains no field', () => {
    const { diagram, adjusted } = san({ nodes: [], edges: [], notes: [] })
    expect(adjusted).toBe(false)
    expect('shapes' in diagram).toBe(false)
  })
  it('caps the count', () => {
    const many = Array.from({ length: 50 }, (_, i) => ({ id: `s${i}`, kind: 'ellipse', x: 0, y: 0, w: 50, h: 50 }))
    expect(san({ nodes: [], edges: [], notes: [], shapes: many }).diagram.shapes).toHaveLength(40)
  })
  it('shares the id space with groups', () => {
    const { diagram } = san({ nodes: [], edges: [], notes: [], groups: [{ id: 'x', kind: 'az', label: '', x: 0, y: 0, w: 100, h: 100 }], shapes: [{ id: 'x', kind: 'ellipse', x: 0, y: 0, w: 50, h: 50 }] })
    expect(diagram.shapes).toBeUndefined()
  })
})

describe('shapes in copy and paste and templates', () => {
  const d = { nodes: [], edges: [], notes: [], shapes: [newShape('s1', 'arrow', 10, 10), newShape('s2', 'ellipse', 300, 300)] }
  it('copies only the chosen shapes and pastes them with fresh ids, offset', () => {
    const clip = copySelection(d, ['s1'])
    expect(clip.shapes).toHaveLength(1)
    let n = 0
    const out = pasteClip(d, clip, (p) => `${p}new${++n}`)
    expect(out.diagram.shapes).toHaveLength(3)
    expect(out.diagram.shapes![2]).toMatchObject({ id: 'snew1', x: 34, y: 34, kind: 'arrow' })
    expect(out.ids).toEqual(['snew1'])
    expect(out.diagram.shapes![2].w).toBe(d.shapes[0].w)
  })
  it('a saved template is moved to the origin including its shapes', () => {
    const t = addUserTemplate([], 'x', d, 'id')
    expect(Math.min(...t[0].diagram.shapes!.map((s) => s.x))).toBe(0)
  })
})
