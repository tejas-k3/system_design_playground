import type { Rect } from '../draw/geometry'
import { groupInfo } from './groups'

/** Approximate width of one character of the map's 7px label text. */
export const MINI_CHAR = 4.2

/** The text to print in a box's outline on the map: "AZ: label" if it fits, the short tag if only that fits, else nothing. */
export function miniTag(g: { kind: string; label: string }, width: number): string {
  const info = groupInfo(g.kind)
  const full = g.label ? `${info.tag}: ${g.label}` : info.tag
  const room = width - 6
  if (full.length * MINI_CHAR <= room) return full
  if (info.tag.length * MINI_CHAR <= room) return info.tag
  return ''
}
import type { Viewport } from '../state/store'

export interface MiniLayout {
  /** map pixels per world unit */
  scale: number
  /** where the world's (0,0) is in the map */
  originX: number
  originY: number
  nodes: Rect[]
  /** what the canvas currently shows */
  view: Rect
}

/**
 * Places the design (`rects`, in world units) and the canvas's current view in a small map. The map covers the
 * design and the view together, so the view rectangle stays inside the map however far the canvas is scrolled.
 */
export function miniLayout(rects: Rect[], vp: Viewport, canvas: { w: number; h: number }, map: { w: number; h: number }, pad = 6): MiniLayout {
  const viewWorld: Rect = { x: -vp.scrollX / vp.zoom, y: -vp.scrollY / vp.zoom, w: canvas.w / vp.zoom, h: canvas.h / vp.zoom }
  const all = [...rects, viewWorld]
  const minX = Math.min(...all.map((r) => r.x))
  const minY = Math.min(...all.map((r) => r.y))
  const maxX = Math.max(...all.map((r) => r.x + r.w))
  const maxY = Math.max(...all.map((r) => r.y + r.h))
  const w = Math.max(1, maxX - minX)
  const h = Math.max(1, maxY - minY)
  const scale = Math.min((map.w - pad * 2) / w, (map.h - pad * 2) / h)
  const originX = (map.w - w * scale) / 2 - minX * scale
  const originY = (map.h - h * scale) / 2 - minY * scale
  const to = (r: Rect): Rect => ({ x: originX + r.x * scale, y: originY + r.y * scale, w: r.w * scale, h: r.h * scale })
  return { scale, originX, originY, nodes: rects.map(to), view: to(viewWorld) }
}

/** The viewport after a click on the map: the clicked spot moves to the middle of the canvas, the zoom stays. */
export function miniToViewport(click: { x: number; y: number }, l: MiniLayout, canvas: { w: number; h: number }, zoom: number): Viewport {
  const wx = (click.x - l.originX) / l.scale
  const wy = (click.y - l.originY) / l.scale
  return { zoom, scrollX: canvas.w / 2 - wx * zoom, scrollY: canvas.h / 2 - wy * zoom }
}
