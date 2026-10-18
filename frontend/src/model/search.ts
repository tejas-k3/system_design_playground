import type { Diagram, Node } from '../api/types'
import type { Rect } from '../draw/geometry'
import type { Viewport } from '../state/store'

/**
 * Components that match a search: every word of the query must appear in the label or the component's type
 * name, ignoring case. Labels that start with the first word come first, then labels that contain it, then
 * matches on the type only; each group keeps the diagram's order. An empty query lists everything.
 */
export function searchNodes(d: Diagram, kindName: (kind: string) => string, query: string): Node[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (words.length === 0) return d.nodes
  const rank = (n: Node): number => {
    const label = n.label.toLowerCase()
    const hay = `${label} ${kindName(n.kind).toLowerCase()}`
    if (!words.every((w) => hay.includes(w))) return -1
    if (label.startsWith(words[0])) return 0
    return words.every((w) => label.includes(w)) ? 1 : 2
  }
  return d.nodes
    .map((n, i) => ({ n, i, r: rank(n) }))
    .filter((x) => x.r >= 0)
    .sort((a, b) => a.r - b.r || a.i - b.i)
    .map((x) => x.n)
}

const MIN_READABLE_ZOOM = 0.6

/** The viewport that puts the middle of `r` in the middle of the canvas, zooming in if the view is too small to read. */
export function centerOn(r: Rect, size: { w: number; h: number }, v: Viewport): Viewport {
  const zoom = Math.max(v.zoom, MIN_READABLE_ZOOM)
  return { zoom, scrollX: size.w / 2 - (r.x + r.w / 2) * zoom, scrollY: size.h / 2 - (r.y + r.h / 2) * zoom }
}
