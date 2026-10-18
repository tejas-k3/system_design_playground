import { type RefObject, useRef } from 'react'
import { nodeRect } from '../../draw/geometry'
import { miniLayout, miniToViewport } from '../../model/minimap'
import { useStore } from '../../state/store'

const MAP = { w: 180, h: 110 }

/**
 * A small overview of the whole design with the part the canvas shows outlined. Click or drag on it to move
 * the view. It is a pointer shortcut only: the keyboard already has fit (F), zoom and find (Ctrl/Cmd+K).
 */
export function Minimap({ wrap }: { wrap: RefObject<HTMLElement | null> }) {
  const diagram = useStore((s) => s.diagram)
  const reg = useStore((s) => s.reg)
  const vp = useStore((s) => s.viewport)
  const setViewport = useStore((s) => s.setViewport)
  const drag = useRef(false)
  const el = wrap.current
  if (!reg || diagram.nodes.length < 4 || !el) return null // a handful of components fits on screen without a map
  const { width, height } = el.getBoundingClientRect()
  const canvas = { w: width, h: height }
  const l = miniLayout(diagram.nodes.map((n) => nodeRect(n, reg)), vp, canvas, MAP)
  const move = (e: React.PointerEvent<SVGSVGElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    setViewport(miniToViewport({ x: e.clientX - r.left, y: e.clientY - r.top }, l, canvas, vp.zoom))
  }
  return (
    <svg
      className="minimap"
      width={MAP.w}
      height={MAP.h}
      role="img"
      aria-label="Overview map of the design; click or drag to move the view"
      onPointerDown={(e) => { drag.current = true; e.currentTarget.setPointerCapture(e.pointerId); move(e) }}
      onPointerMove={(e) => { if (drag.current) move(e) }}
      onPointerUp={() => { drag.current = false }}
      onPointerCancel={() => { drag.current = false }}
    >
      {l.nodes.map((r, i) => <rect key={diagram.nodes[i].id} x={r.x} y={r.y} width={Math.max(2, r.w)} height={Math.max(2, r.h)} rx={1.5} className="mm-node" />)}
      <rect x={l.view.x} y={l.view.y} width={l.view.w} height={l.view.h} className="mm-view" />
    </svg>
  )
}
