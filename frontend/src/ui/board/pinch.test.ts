import { describe, expect, it } from 'vitest'
import { pinchOf, pinchStep } from './pinch'

describe('pinch', () => {
  it('measures the distance and midpoint of two fingers', () => {
    expect(pinchOf({ x: 0, y: 0 }, { x: 30, y: 40 })).toEqual({ dist: 50, mid: { x: 15, y: 20 } })
  })

  it('zooms by the change in finger distance and does not pan when the midpoint stays put', () => {
    const prev = pinchOf({ x: 100, y: 100 }, { x: 200, y: 100 })
    const next = pinchOf({ x: 50, y: 100 }, { x: 250, y: 100 })
    const s = pinchStep(prev, next)
    expect(s.factor).toBe(2)
    expect(s.dx).toBe(0)
    expect(s.dy).toBe(0)
  })

  it('pans by the movement of the midpoint when the fingers keep their spacing', () => {
    const prev = pinchOf({ x: 100, y: 100 }, { x: 200, y: 100 })
    const next = pinchOf({ x: 130, y: 90 }, { x: 230, y: 90 })
    expect(pinchStep(prev, next)).toEqual({ dx: 30, dy: -10, factor: 1 })
  })

  it('pinching in zooms out', () => {
    expect(pinchStep(pinchOf({ x: 0, y: 0 }, { x: 200, y: 0 }), pinchOf({ x: 50, y: 0 }, { x: 150, y: 0 })).factor).toBe(0.5)
  })

  it('ignores a degenerate start where the fingers touch', () => {
    expect(pinchStep(pinchOf({ x: 5, y: 5 }, { x: 5, y: 5 }), pinchOf({ x: 0, y: 0 }, { x: 100, y: 0 })).factor).toBe(1)
  })
})
