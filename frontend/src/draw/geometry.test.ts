import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import type { Reg } from '../model/diagram'
import { HANDLE_R, particleShape, fitFontSize, handlePos, handleRadius, hitTest, legibleSize, nodeRect } from './geometry'

const reg = { spec: () => ({ width: 160, height: 72 }) } as unknown as Reg
const diagram = { nodes: [{ id: 'a', kind: 'appServer', x: 0, y: 0, label: 'a', params: {} }], edges: [], notes: [] } as unknown as Diagram
const measure = () => ({ w: 10, h: 10 })

describe('connection handle', () => {
  it('keeps a comfortable size on screen however far the view is zoomed out', () => {
    expect(handleRadius(1)).toBe(12)
    expect(handleRadius(2)).toBe(HANDLE_R) // zoomed in it is already big enough on screen
    expect(handleRadius(0.5) * 0.5).toBeGreaterThanOrEqual(12)
    expect(handleRadius(0.25) * 0.25).toBeGreaterThanOrEqual(12)
  })

  it('can be grabbed 10 screen pixels from its centre when zoomed out', () => {
    const zoom = 0.5
    const h = handlePos(nodeRect(diagram.nodes[0], reg), zoom)
    const hit = hitTest({ x: h.x + 10 / zoom, y: h.y }, diagram, reg, zoom, measure, new Set(['a']))
    expect(hit).toEqual({ type: 'handle', id: 'a' })
  })

  it('does not reach into the component it belongs to', () => {
    const zoom = 0.25
    const r = nodeRect(diagram.nodes[0], reg)
    expect(handlePos(r, zoom).x - handleRadius(zoom)).toBeGreaterThanOrEqual(r.x + r.w)
  })
})

describe('legibleSize', () => {
  it('leaves text alone while it is big enough on screen', () => {
    expect(legibleSize(13, 1, 10)).toBe(13)
    expect(legibleSize(13, 0.7, 9)).toBe(13)
  })
  it('grows text in canvas units so it keeps a minimum size on screen when zoomed out', () => {
    expect(legibleSize(13, 0.6, 10) * 0.6).toBeCloseTo(10)
  })
  it('never more than one and a half times the design size, so it still fits its box', () => {
    expect(legibleSize(13, 0.1, 10)).toBe(19.5)
  })
})

describe('fitFontSize', () => {
  const measure = (text: string, size: number) => text.length * size * 0.5
  it('keeps the preferred size when the text fits', () => {
    expect(fitFontSize(measure, 'Cache', 100, 16, 12)).toBe(16)
  })
  it('shrinks to fit rather than truncating', () => {
    const size = fitFontSize(measure, 'Load Balancer', 98, 16, 12)
    expect(size).toBeLessThan(16)
    expect(measure('Load Balancer', size)).toBeLessThanOrEqual(98)
  })
  it('stops at the minimum, where the caller truncates', () => {
    expect(fitFontSize(measure, 'A very long component name indeed', 98, 16, 12)).toBe(12)
  })
})

describe('particleShape', () => {
  it('draws a failed request as a cross and everything else as a dot, so failure never depends on colour alone', () => {
    expect(particleShape({ fail: true, back: false })).toBe('cross')
    expect(particleShape({ fail: true, back: true })).toBe('cross')
    expect(particleShape({ fail: false, back: true })).toBe('dot')
    expect(particleShape({ fail: false, back: false })).toBe('dot')
  })
})
