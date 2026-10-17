import type { Diagram, Edge, Node, Note } from '../api/types'
import type { Reg } from '../model/diagram'

export interface Rect {
  x: number
  y: number
  w: number
  h: number
}
export interface Pt {
  x: number
  y: number
}

export const HANDLE_R = 8
/** On-screen radius the connection handle never drops below, so it stays easy to see and grab when zoomed out. */
const HANDLE_MIN_SCREEN_R = 12
export const NOTE_FONT = 18

export function nodeRect(n: Node, reg: Reg): Rect {
  const s = reg.spec(n.kind)
  return { x: n.x, y: n.y, w: s?.width ?? 160, h: s?.height ?? 72 }
}

export const center = (r: Rect): Pt => ({ x: r.x + r.w / 2, y: r.y + r.h / 2 })
export const handleRadius = (zoom: number): number => Math.max(HANDLE_R, HANDLE_MIN_SCREEN_R / zoom)
export const handlePos = (r: Rect, zoom: number): Pt => ({ x: r.x + r.w + handleRadius(zoom) + 2, y: r.y + r.h / 2 })

export function inRect(p: Pt, r: Rect, pad = 0): boolean {
  return p.x >= r.x - pad && p.x <= r.x + r.w + pad && p.y >= r.y - pad && p.y <= r.y + r.h + pad
}

export function rectsIntersect(a: Rect, b: Rect): boolean {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y
}

/** Point where the ray from the centre of r toward `toward` leaves r, pushed out by `gap`. */
export function boundaryPoint(r: Rect, toward: Pt, gap = 0): Pt {
  const c = center(r)
  const dx = toward.x - c.x
  const dy = toward.y - c.y
  if (dx === 0 && dy === 0) return c
  const sx = dx === 0 ? Infinity : r.w / 2 / Math.abs(dx)
  const sy = dy === 0 ? Infinity : r.h / 2 / Math.abs(dy)
  const t = Math.min(sx, sy)
  const len = Math.hypot(dx, dy)
  return { x: c.x + dx * t + (dx / len) * gap, y: c.y + dy * t + (dy / len) * gap }
}

export interface EdgeGeom {
  a: Pt
  b: Pt
}

export function edgeGeom(e: Edge, nodes: Map<string, Node>, reg: Reg): EdgeGeom | null {
  const f = nodes.get(e.from)
  const t = nodes.get(e.to)
  if (!f || !t) return null
  const fr = nodeRect(f, reg)
  const tr = nodeRect(t, reg)
  return { a: boundaryPoint(fr, center(tr), 4), b: boundaryPoint(tr, center(fr), 6) }
}

export function distToSegment(p: Pt, a: Pt, b: Pt): number {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const l2 = dx * dx + dy * dy
  if (l2 === 0) return Math.hypot(p.x - a.x, p.y - a.y)
  const t = Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / l2))
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy))
}

export function noteRect(n: Note, measure: (text: string, size: number) => { w: number; h: number }): Rect {
  const size = n.fontSize ?? NOTE_FONT
  const m = measure(n.text || 'Note', size)
  return { x: n.x, y: n.y, w: m.w + 16, h: m.h + 12 }
}

export type Hit =
  | { type: 'handle'; id: string }
  | { type: 'node'; id: string }
  | { type: 'note'; id: string }
  | { type: 'edge'; id: string }
  | null

export function hitTest(
  p: Pt,
  d: Diagram,
  reg: Reg,
  zoom: number,
  measure: (text: string, size: number) => { w: number; h: number },
  handleFor: Set<string>,
): Hit {
  const byId = new Map(d.nodes.map((n) => [n.id, n]))
  for (const id of handleFor) {
    const n = byId.get(id)
    if (n) {
      const h = handlePos(nodeRect(n, reg), zoom)
      if (Math.hypot(p.x - h.x, p.y - h.y) <= handleRadius(zoom) + 4 / zoom) return { type: 'handle', id }
    }
  }
  for (let i = d.notes.length - 1; i >= 0; i--) if (inRect(p, noteRect(d.notes[i], measure))) return { type: 'note', id: d.notes[i].id }
  for (let i = d.nodes.length - 1; i >= 0; i--) if (inRect(p, nodeRect(d.nodes[i], reg))) return { type: 'node', id: d.nodes[i].id }
  let best: { id: string; dist: number } | null = null
  const tol = 8 / zoom
  for (const e of d.edges) {
    const g = edgeGeom(e, byId, reg)
    if (!g) continue
    const dist = distToSegment(p, g.a, g.b)
    if (dist <= tol && (!best || dist < best.dist)) best = { id: e.id, dist }
  }
  return best ? { type: 'edge', id: best.id } : null
}

export function worldToScreen(p: Pt, v: { scrollX: number; scrollY: number; zoom: number }): Pt {
  return { x: p.x * v.zoom + v.scrollX, y: p.y * v.zoom + v.scrollY }
}
export function screenToWorld(p: Pt, v: { scrollX: number; scrollY: number; zoom: number }): Pt {
  return { x: (p.x - v.scrollX) / v.zoom, y: (p.y - v.scrollY) / v.zoom }
}

export function boundsOf(d: Diagram, reg: Reg, measure: (t: string, s: number) => { w: number; h: number }): Rect | null {
  const rs: Rect[] = [...d.nodes.map((n) => nodeRect(n, reg)), ...d.notes.map((n) => noteRect(n, measure))]
  if (!rs.length) return null
  const x1 = Math.min(...rs.map((r) => r.x))
  const y1 = Math.min(...rs.map((r) => r.y))
  const x2 = Math.max(...rs.map((r) => r.x + r.w))
  const y2 = Math.max(...rs.map((r) => r.y + r.h))
  return { x: x1, y: y1, w: x2 - x1, h: y2 - y1 }
}
