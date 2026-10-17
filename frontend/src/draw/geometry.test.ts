import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import type { Reg } from '../model/diagram'
import { HANDLE_R, handlePos, handleRadius, hitTest, nodeRect } from './geometry'

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
