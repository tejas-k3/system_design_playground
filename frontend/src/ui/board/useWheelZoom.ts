import { type RefObject, useEffect } from 'react'
import { useStore } from '../../state/store'

/**
 * Zoom around the cursor on mouse wheel or trackpad pinch. The listener is attached natively
 * as non-passive so it can stop the page from scrolling, which React's onWheel cannot do.
 */
export function useWheelZoom(canvas: RefObject<HTMLCanvasElement | null>): void {
  useEffect(() => {
    const el = canvas.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      const r = el.getBoundingClientRect()
      const factor = Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.0015))
      useStore.getState().zoomAt(factor, e.clientX - r.left, e.clientY - r.top)
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [canvas])
}
