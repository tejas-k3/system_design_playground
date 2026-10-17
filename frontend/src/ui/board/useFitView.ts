import { type RefObject, useCallback, useEffect } from 'react'
import { boundsOf } from '../../draw/geometry'
import { measureText } from '../../draw/renderer'
import { fitViewport } from '../../model/fit'
import { isNarrow } from '../../model/responsive'
import { useStore } from '../../state/store'
import type { CanvasSize } from './useCanvasSize'

const PADDING = 80
const PADDING_NARROW = 24 // phones have no room to spare

/**
 * Returns a function that zooms and centres the viewport on the whole design, and runs it when
 * a fit is requested (toolbar, F key) or a different challenge is opened.
 */
export function useFitView(size: RefObject<CanvasSize>): () => void {
  const fit = useCallback(() => {
    const st = useStore.getState()
    if (!st.reg) return
    st.setViewport(fitViewport(boundsOf(st.diagram, st.reg, measureText), size.current, isNarrow() ? PADDING_NARROW : PADDING))
  }, [size])

  const fitRequest = useStore((s) => s.fitRequest)
  const scenarioId = useStore((s) => s.scenario?.def.id)
  useEffect(() => {
    fit()
  }, [fitRequest, scenarioId, fit])

  return fit
}
