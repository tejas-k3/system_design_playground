import { describe, expect, it } from 'vitest'
import { miniLayout, miniTag, miniToViewport } from './minimap'

describe('miniTag', () => {
  it('shows the tag and label when the box is wide enough', () => {
    expect(miniTag({ kind: 'az', label: 'us-east-1a' }, 120)).toBe('AZ: us-east-1a')
  })
  it('falls back to the short tag, then to nothing', () => {
    expect(miniTag({ kind: 'az', label: 'us-east-1a' }, 24)).toBe('AZ')
    expect(miniTag({ kind: 'az', label: 'us-east-1a' }, 6)).toBe('')
  })
})

const map = { w: 200, h: 120 }
const bounds = { x: 0, y: 0, w: 1000, h: 400 }
const view = { scrollX: 0, scrollY: 0, zoom: 1 }
const canvas = { w: 800, h: 500 }

describe('miniLayout', () => {
  it('scales the design to fit the map with the same proportions and centres it', () => {
    // a view inside the design, so the map covers just the design
    const l = miniLayout([bounds], view, { w: 400, h: 200 }, map)
    expect(l.scale).toBeCloseTo(0.188, 2) // 200/1000 limits width; 120/400 would allow 0.3
    expect(l.nodes[0].w).toBeCloseTo(1000 * l.scale)
    expect(l.nodes[0].x + l.nodes[0].w / 2).toBeCloseTo(map.w / 2)
    expect(l.nodes[0].y + l.nodes[0].h / 2).toBeCloseTo(map.h / 2)
  })
  it('the viewport rectangle covers what the canvas shows, in map units', () => {
    const l = miniLayout([bounds], { scrollX: -100, scrollY: -50, zoom: 2 }, canvas, map)
    // the canvas shows world x 50..450 and y 25..275 at zoom 2
    const s = l.scale
    expect(l.view.w).toBeCloseTo(400 * s)
    expect(l.view.h).toBeCloseTo(250 * s)
    expect(l.view.x - l.nodes[0].x).toBeCloseTo(50 * s)
  })
  it('includes the viewport in what the map has to cover when it is scrolled away from the design', () => {
    const l = miniLayout([bounds], { scrollX: -5000, scrollY: 0, zoom: 1 }, canvas, map)
    expect(l.view.x).toBeGreaterThanOrEqual(0)
    expect(l.view.x + l.view.w).toBeLessThanOrEqual(map.w + 0.001)
  })
  it('handles an empty design', () => {
    const l = miniLayout([], view, canvas, map)
    expect(l.nodes).toEqual([])
    expect(Number.isFinite(l.scale)).toBe(true)
  })
})

describe('miniToViewport', () => {
  it('a click puts that spot in the middle of the canvas and keeps the zoom', () => {
    const l = miniLayout([bounds], { scrollX: 0, scrollY: 0, zoom: 1.5 }, canvas, map)
    const click = { x: l.nodes[0].x + 500 * l.scale, y: l.nodes[0].y + 200 * l.scale } // world (500, 200)
    const v = miniToViewport(click, l, canvas, 1.5)
    expect(v.zoom).toBe(1.5)
    expect(500 * 1.5 + v.scrollX).toBeCloseTo(canvas.w / 2)
    expect(200 * 1.5 + v.scrollY).toBeCloseTo(canvas.h / 2)
  })
})
