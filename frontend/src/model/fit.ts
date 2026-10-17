import type { Rect } from '../draw/geometry'
import type { Viewport } from '../state/store'
import { MIN_ZOOM } from './diagram'

export const MAX_FIT_ZOOM = 1.2

/**
 * The viewport that centres `bounds` (the whole design, in world units) inside a canvas of `size`
 * pixels with `pad` pixels to spare on every side. Zoom is limited so a tiny design is not blown up
 * and a huge one stays usable; an empty design or a canvas with no size gets the default view.
 */
export function fitViewport(bounds: Rect | null, size: { w: number; h: number }, pad: number): Viewport {
  if (!bounds || size.w <= 0 || size.h <= 0) return { scrollX: 0, scrollY: 0, zoom: 1 }
  const zoom = Math.min(MAX_FIT_ZOOM, Math.max(MIN_ZOOM, Math.min((size.w - pad * 2) / bounds.w, (size.h - pad * 2) / bounds.h)))
  return {
    zoom,
    scrollX: (size.w - bounds.w * zoom) / 2 - bounds.x * zoom,
    scrollY: (size.h - bounds.h * zoom) / 2 - bounds.y * zoom,
  }
}
