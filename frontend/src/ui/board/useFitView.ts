import { type RefObject, useCallback, useEffect } from 'react'
import { boundsOf } from '../../draw/geometry'
import { measureText } from '../../draw/renderer'
import { MIN_ZOOM } from '../../model/diagram'
import { isNarrow } from '../../model/responsive'
import { useStore } from '../../state/store'
import type { CanvasSize } from './useCanvasSize'

const PADDING = 80
const PADDING_NARROW = 24 // phones have no room to spare
const MAX_FIT_ZOOM = 1.2

/**
 * Returns a function that zooms and centres the viewport on the whole design, and runs it when
 * a fit is requested (toolbar, F key) or a different challenge is opened.
 */
export function useFitView(size: RefObject<CanvasSize>): () => void {
  const fit = useCallback(() => {
    const st = useStore.getState()
    if (!st.reg) return
    const b = boundsOf(st.diagram, st.reg, measureText)
    const { w, h } = size.current
    if (!b || !w) {
      st.setViewport({ scrollX: 0, scrollY: 0, zoom: 1 })
      return
    }
    const pad = isNarrow() ? PADDING_NARROW : PADDING
    const zoom = Math.min(MAX_FIT_ZOOM, Math.max(MIN_ZOOM, Math.min((w - pad * 2) / b.w, (h - pad * 2) / b.h)))
    st.setViewport({ zoom, scrollX: (w - b.w * zoom) / 2 - b.x * zoom, scrollY: (h - b.h * zoom) / 2 - b.y * zoom })
  }, [size])

  const fitRequest = useStore((s) => s.fitRequest)
  const scenarioId = useStore((s) => s.scenario?.def.id)
  useEffect(() => {
    fit()
  }, [fitRequest, scenarioId, fit])

  return fit
}
