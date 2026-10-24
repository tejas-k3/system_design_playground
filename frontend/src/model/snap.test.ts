import { describe, expect, it } from 'vitest'
import { snapMove, unionRect } from './snap'

const r = (x: number, y: number, w = 100, h = 50) => ({ x, y, w, h })

describe('snapMove', () => {
  it('does nothing when no edge or centre is within reach', () => {
    const s = snapMove(r(0, 0), [r(300, 300)], 6)
    expect(s).toEqual({ dx: 0, dy: 0, guides: [] })
  })

  it('pulls the left edge onto a neighbour\'s left edge', () => {
    const s = snapMove(r(203, 0), [r(200, 200)], 6)
    expect(s.dx).toBe(-3)
    expect(s.dy).toBe(0)
    expect(s.guides).toEqual([{ axis: 'x', pos: 200, from: 0, to: 250 }])
  })

  it('pulls centres together', () => {
    // moving centre x = 53 + 50 = 103; neighbour centre x = 100 + 50 = 150 -> 47 apart: no. neighbour at x=50: centre 100 -> 3 apart
    const s = snapMove(r(53, 0), [r(50, 200)], 6)
    expect(s.dx).toBe(-3)
    // y: centres 25 vs 225 are far apart, nothing else lines up
    expect(s.dy).toBe(0)
  })

  it('a right edge can meet a left edge (butting up against a neighbour)', () => {
    const s = snapMove(r(104, 400), [r(200, 0)], 6) // moving right edge 204 vs neighbour left edge 200
    expect(s.dx).toBe(-4)
  })

  it('snaps both axes at once and gives a guide for each', () => {
    const s = snapMove(r(203, 197), [r(200, 200)], 6)
    expect([s.dx, s.dy]).toEqual([-3, 3])
    expect(s.guides.map((g) => g.axis).sort()).toEqual(['x', 'y'])
  })

  it('picks the nearest of several candidates', () => {
    const s = snapMove(r(104, 0), [r(100, 200), r(106, 400)], 6)
    expect(s.dx).toBe(2) // 104 is 4 from 100 and 2 from 106: the nearer wins
  })

  it('ignores neighbours when the threshold is zero', () => {
    expect(snapMove(r(203, 0), [r(200, 200)], 0)).toEqual({ dx: 0, dy: 0, guides: [] })
  })

  it('the guide spans both rectangles so it can be seen joining them', () => {
    const s = snapMove(r(200, 0), [r(200, 400)], 6)
    expect(s.guides[0]).toEqual({ axis: 'x', pos: 200, from: 0, to: 450 })
  })

  it('unionRect bounds several rects', () => {
    expect(unionRect([r(0, 0), r(200, 100)])).toEqual({ x: 0, y: 0, w: 300, h: 150 })
    expect(unionRect([])).toBeNull()
  })
})
