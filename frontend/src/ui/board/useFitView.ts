import { type RefObject, useCallback, useEffect } from 'react'
import { boundsOf } from '../../draw/geometry'
import { measureText } from '../../draw/renderer'
import { fitViewport } from '../../model/fit'
import { centerOn } from '../../model/search'
import { nodeRect } from '../../draw/geometry'
import { isNarrow } from '../../model/responsive'
import { useStore } from '../../state/store'

const PADDING = 80
const PADDING_NARROW = 24 // phones have no room to spare

/**
 * Returns a function that zooms and centres the viewport on the whole design, and runs it when
 * a fit is requested (toolbar, F key) or a different challenge is opened.
 *
 * The canvas is measured at the moment of fitting, not read from a cached size: opening a challenge
 * mounts a side panel in the same render that triggers the fit, and the resize that follows has not
 * been observed yet, so a cached size is still the old, wider one and the design ends up clipped.
 */
export function useFitView(wrap: RefObject<HTMLElement | null>): () => void {
  const fit = useCallback(() => {
    const st = useStore.getState()
    const el = wrap.current
    if (!st.reg || !el) return
    const { width, height } = el.getBoundingClientRect()
    st.setViewport(fitViewport(boundsOf(st.diagram, st.reg, measureText), { w: width, h: height }, isNarrow() ? PADDING_NARROW : PADDING))
  }, [wrap])

  const fitRequest = useStore((s) => s.fitRequest)
  const scenarioId = useStore((s) => s.scenario?.def.id)
  useEffect(() => {
    fit()
  }, [fitRequest, scenarioId, fit])

  const centerRequest = useStore((s) => s.centerRequest)
  useEffect(() => {
    const st = useStore.getState()
    const el = wrap.current
    const n = centerRequest && st.diagram.nodes.find((x) => x.id === centerRequest.id)
    if (!n || !st.reg || !el) return
    const { width, height } = el.getBoundingClientRect()
    st.setViewport(centerOn(nodeRect(n, st.reg), { w: width, h: height }, st.viewport))
  }, [centerRequest, wrap])

  return fit
}
