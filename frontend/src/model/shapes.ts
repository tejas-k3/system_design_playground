import type { Diagram, Shape, ShapeKind } from '../api/types'
import type { Pt, Rect } from '../draw/geometry'

/**
 * Shapes are free annotations drawn over the boxes: an ellipse to circle something, an arrow to point at it.
 * Like notes and boxes they are visual only; the simulation ignores them. A caption is optional and drawn as text,
 * and the two kinds differ in form (round outline versus a line with a head), never in colour alone.
 */
export interface ShapeKindInfo {
  kind: ShapeKind
  name: string
  hint: string
}
export const SHAPE_KINDS: ShapeKindInfo[] = [
  { kind: 'ellipse', name: 'Ellipse', hint: 'Circle a part of the design' },
  { kind: 'arrow', name: 'Arrow', hint: 'Point at something (not a connection)' },
]
export const SHAPE_KIND_IDS: string[] = SHAPE_KINDS.map((k) => k.kind)
export const MAX_SHAPES = 40
export const MIN_SHAPE = 20
export const MAX_SHAPE = 4000
export const MAX_SHAPE_LABEL = 60
export const ELLIPSE_DEFAULT = { w: 220, h: 130 }
export const ARROW_DEFAULT = { w: 200, h: 0 }
/** Screen-pixel radius of a selected shape's grips (ellipse corner, arrow ends). */
export const SHAPE_GRIP = 10

export const shapesOf = (d: Diagram): Shape[] => d.shapes ?? []
export const shapeInfo = (kind: string): ShapeKindInfo => SHAPE_KINDS.find((k) => k.kind === kind) ?? SHAPE_KINDS[0]
/** The text a screen reader shows: "Arrow: reads go here" or just "Arrow". */
export const shapeTitle = (s: Shape): string => (s.label ? `${shapeInfo(s.kind).name}: ${s.label}` : shapeInfo(s.kind).name)

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

/** Brings w and h into range: an ellipse has a positive size, an arrow a signed extent that is never zero. */
export function clampShape(kind: string, w: number, h: number): { w: number; h: number } {
  if (kind === 'arrow') {
    const aw = clamp(w, -MAX_SHAPE, MAX_SHAPE)
    const ah = clamp(h, -MAX_SHAPE, MAX_SHAPE)
    return Math.hypot(aw, ah) < 1 ? { w: MIN_SHAPE, h: ah } : { w: aw, h: ah }
  }
  return { w: clamp(w, MIN_SHAPE, MAX_SHAPE), h: clamp(h, MIN_SHAPE, MAX_SHAPE) }
}

/** The two ends of an arrow in world units. */
export const arrowEnds = (s: Shape): { a: Pt; b: Pt } => ({ a: { x: s.x, y: s.y }, b: { x: s.x + s.w, y: s.y + s.h } })

/** The box around a shape (normalised, so an arrow pointing up-left still has a positive size). */
export function shapeRect(s: Shape): Rect {
  return { x: Math.min(s.x, s.x + s.w), y: Math.min(s.y, s.y + s.h), w: Math.abs(s.w), h: Math.abs(s.h) }
}

const distToSegment = (p: Pt, a: Pt, b: Pt): number => {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const l2 = dx * dx + dy * dy
  if (l2 === 0) return Math.hypot(p.x - a.x, p.y - a.y)
  const t = Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / l2))
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy))
}

/** Whether a point is on the shape's line: the ellipse's outline or the arrow's shaft, within `tol` world units. */
export function onShape(p: Pt, s: Shape, tol: number): boolean {
  if (s.kind === 'arrow') {
    const { a, b } = arrowEnds(s)
    return distToSegment(p, a, b) <= tol
  }
  const rx = s.w / 2
  const ry = s.h / 2
  const q = Math.hypot((p.x - (s.x + rx)) / rx, (p.y - (s.y + ry)) / ry)
  return Math.abs(q - 1) * Math.min(rx, ry) <= tol
}

/** A fresh shape of this kind with no caption. */
export const newShape = (id: string, kind: ShapeKind, x: number, y: number): Shape => ({ id, kind, label: '', x, y, ...(kind === 'arrow' ? ARROW_DEFAULT : ELLIPSE_DEFAULT) })
