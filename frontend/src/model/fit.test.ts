import { describe, expect, it } from 'vitest'
import { MAX_FIT_ZOOM, fitViewport } from './fit'

const screen = (v: ReturnType<typeof fitViewport>, x: number, y: number) => ({ x: x * v.zoom + v.scrollX, y: y * v.zoom + v.scrollY })

describe('fitViewport', () => {
  it('puts every corner of the design inside the canvas, with the padding to spare', () => {
    const bounds = { x: -200, y: 40, w: 1120, h: 300 }
    for (const size of [{ w: 908, h: 640 }, { w: 568, h: 640 }, { w: 390, h: 700 }, { w: 1440, h: 400 }]) {
      const v = fitViewport(bounds, size, 24)
      const tl = screen(v, bounds.x, bounds.y)
      const br = screen(v, bounds.x + bounds.w, bounds.y + bounds.h)
      expect(tl.x).toBeGreaterThanOrEqual(24 - 1e-6)
      expect(tl.y).toBeGreaterThanOrEqual(24 - 1e-6)
      expect(br.x).toBeLessThanOrEqual(size.w - 24 + 1e-6)
      expect(br.y).toBeLessThanOrEqual(size.h - 24 + 1e-6)
    }
  })

  it('centres the design', () => {
    const bounds = { x: 100, y: 100, w: 400, h: 200 }
    const size = { w: 1000, h: 800 }
    const v = fitViewport(bounds, size, 40)
    const tl = screen(v, bounds.x, bounds.y)
    const br = screen(v, bounds.x + bounds.w, bounds.y + bounds.h)
    expect(tl.x).toBeCloseTo(size.w - br.x)
    expect(tl.y).toBeCloseTo(size.h - br.y)
  })

  it('fits a narrower canvas more tightly: a smaller canvas can never give a larger zoom', () => {
    const bounds = { x: 0, y: 0, w: 1120, h: 300 }
    const wide = fitViewport(bounds, { w: 908, h: 640 }, 80).zoom
    const narrow = fitViewport(bounds, { w: 568, h: 640 }, 80).zoom
    expect(narrow).toBeLessThan(wide)
  })

  it('does not blow a small design up past the maximum, nor shrink a huge one to nothing', () => {
    expect(fitViewport({ x: 0, y: 0, w: 50, h: 50 }, { w: 1000, h: 800 }, 80).zoom).toBe(MAX_FIT_ZOOM)
    expect(fitViewport({ x: 0, y: 0, w: 1e7, h: 1e7 }, { w: 1000, h: 800 }, 80).zoom).toBe(0.1)
  })

  it('gives the default view for an empty design or a canvas with no size yet', () => {
    const none = { scrollX: 0, scrollY: 0, zoom: 1 }
    expect(fitViewport(null, { w: 1000, h: 800 }, 80)).toEqual(none)
    expect(fitViewport({ x: 0, y: 0, w: 10, h: 10 }, { w: 0, h: 0 }, 80)).toEqual(none)
  })
})
