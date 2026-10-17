import { type RefObject, useLayoutEffect, useRef } from 'react'

export interface CanvasSize {
  w: number
  h: number
  dpr: number
}

/**
 * Keep a canvas the size of its container, scaled for the device pixel ratio.
 * Returns a ref holding the current CSS size; onResize runs after every resize so the caller can redraw.
 */
export function useCanvasSize(wrap: RefObject<HTMLElement | null>, canvas: RefObject<HTMLCanvasElement | null>, onResize: () => void) {
  const size = useRef<CanvasSize>({ w: 0, h: 0, dpr: 1 })
  const resized = useRef(onResize)
  resized.current = onResize

  useLayoutEffect(() => {
    const el = wrap.current
    const cv = canvas.current
    if (!el || !cv) return
    const fit = () => {
      const dpr = window.devicePixelRatio || 1
      const r = el.getBoundingClientRect()
      size.current = { w: r.width, h: r.height, dpr }
      cv.width = Math.max(1, Math.round(r.width * dpr))
      cv.height = Math.max(1, Math.round(r.height * dpr))
      cv.style.width = `${r.width}px`
      cv.style.height = `${r.height}px`
      resized.current()
    }
    fit()
    const ro = new ResizeObserver(fit)
    ro.observe(el)
    return () => ro.disconnect()
  }, [wrap, canvas])

  return size
}
