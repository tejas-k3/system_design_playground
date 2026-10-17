import type { Pt } from '../../draw/geometry'

export interface PinchState {
  dist: number
  mid: Pt
}

export const pinchOf = (a: Pt, b: Pt): PinchState => ({ dist: Math.hypot(a.x - b.x, a.y - b.y), mid: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 } })

/**
 * How the view should change as two fingers move from `prev` to `next`: pan by however far their
 * midpoint moved, then zoom around the new midpoint by the change in finger distance.
 */
export function pinchStep(prev: PinchState, next: PinchState): { dx: number; dy: number; factor: number } {
  const factor = prev.dist > 4 ? next.dist / prev.dist : 1 // ignore a degenerate start where the fingers touch
  return { dx: next.mid.x - prev.mid.x, dy: next.mid.y - prev.mid.y, factor }
}
