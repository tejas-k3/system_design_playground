import { type RefObject, useRef } from 'react'
import { nodeRect } from '../../draw/geometry'
import { groupInfo, groupRect, groupsOf } from '../../model/groups'
import { miniLayout, miniTag, miniToViewport } from '../../model/minimap'
import { useStore } from '../../state/store'

const MAP = { w: 180, h: 110 }
/** The box's own dash pattern, scaled down to fit the little map. */
const groupDash = (kind: string) => groupInfo(kind).dash.map((v) => Math.max(1, v / 3)).join(' ')

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
  const boxes = groupsOf(diagram)
  if (!reg || (diagram.nodes.length < 4 && boxes.length === 0) || !el) return null // a handful of components fits on screen without a map
  const { width, height } = el.getBoundingClientRect()
  const canvas = { w: width, h: height }
  const l = miniLayout([...diagram.nodes.map((n) => nodeRect(n, reg)), ...boxes.map(groupRect)], vp, canvas, MAP)
  const nodeMaps = l.nodes.slice(0, diagram.nodes.length)
  const boxMaps = l.nodes.slice(diagram.nodes.length)
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
      aria-label={`Overview map of the design${boxes.length ? ` with ${boxes.length} ${boxes.length === 1 ? 'box' : 'boxes'}` : ''}; click or drag to move the view`}
      onPointerDown={(e) => { drag.current = true; e.currentTarget.setPointerCapture(e.pointerId); move(e) }}
      onPointerMove={(e) => { if (drag.current) move(e) }}
      onPointerUp={() => { drag.current = false }}
      onPointerCancel={() => { drag.current = false }}
    >
      {[...boxes].sort((a, b) => b.w * b.h - a.w * a.h).map((g) => {
        const r = boxMaps[boxes.indexOf(g)]
        const tag = miniTag(g, r.w)
        return (
          <g key={g.id}>
            <rect x={r.x} y={r.y} width={Math.max(2, r.w)} height={Math.max(2, r.h)} rx={2} className="mm-box" strokeDasharray={g.kind === 'service' ? undefined : groupDash(g.kind)} />
            {tag && <text x={r.x + 3} y={r.y + 8} className="mm-tag">{tag}</text>}
          </g>
        )
      })}
      {nodeMaps.map((r, i) => <rect key={diagram.nodes[i].id} x={r.x} y={r.y} width={Math.max(2, r.w)} height={Math.max(2, r.h)} rx={1.5} className="mm-node" />)}
      <rect x={l.view.x} y={l.view.y} width={l.view.w} height={l.view.h} className="mm-view" />
    </svg>
  )
}
