import type { Rect } from '../draw/geometry'

/** A line to draw while dragging: a vertical (`axis: 'x'`) or horizontal (`'y'`) line at `pos`, running from `from` to `to` along the other axis. */
export interface Guide {
  axis: 'x' | 'y'
  pos: number
  from: number
  to: number
}
export interface Snap {
  dx: number
  dy: number
  guides: Guide[]
}

export function unionRect(rs: Rect[]): Rect | null {
  if (rs.length === 0) return null
  const x1 = Math.min(...rs.map((r) => r.x))
  const y1 = Math.min(...rs.map((r) => r.y))
  const x2 = Math.max(...rs.map((r) => r.x + r.w))
  const y2 = Math.max(...rs.map((r) => r.y + r.h))
  return { x: x1, y: y1, w: x2 - x1, h: y2 - y1 }
}

const xs = (r: Rect): number[] => [r.x, r.x + r.w / 2, r.x + r.w]
const ys = (r: Rect): number[] => [r.y, r.y + r.h / 2, r.y + r.h]

/**
 * Where a dragged rectangle should be nudged so one of its edges or its centre lines up with a neighbour's edge or
 * centre, if any is within `threshold` (in world units). Each axis snaps on its own; the guides say what lined up.
 */
export function snapMove(moving: Rect, others: Rect[], threshold: number): Snap {
  const guides: Guide[] = []
  const pick = (mine: number[], theirs: (r: Rect) => number[]) => {
    let best: { delta: number; other: Rect } | null = null
    for (const o of others) {
      for (const t of theirs(o)) {
        for (const m of mine) {
          const delta = t - m
          if (Math.abs(delta) <= threshold && (!best || Math.abs(delta) < Math.abs(best.delta))) best = { delta, other: o }
        }
      }
    }
    return best
  }
  let dx = 0
  let dy = 0
  const bx = threshold > 0 ? pick(xs(moving), xs) : null
  if (bx) {
    dx = bx.delta
    const m = { ...moving, x: moving.x + dx }
    const at = xs(bx.other).find((t) => xs(m).some((v) => Math.abs(v - t) < 1e-6))!
    guides.push({ axis: 'x', pos: at, from: Math.min(m.y, bx.other.y), to: Math.max(m.y + m.h, bx.other.y + bx.other.h) })
  }
  const by = threshold > 0 ? pick(ys(moving), ys) : null
  if (by) {
    dy = by.delta
    const m = { ...moving, x: moving.x + dx, y: moving.y + dy }
    const at = ys(by.other).find((t) => ys(m).some((v) => Math.abs(v - t) < 1e-6))!
    guides.push({ axis: 'y', pos: at, from: Math.min(m.x, by.other.x), to: Math.max(m.x + m.w, by.other.x + by.other.w) })
  }
  return { dx, dy, guides }
}
