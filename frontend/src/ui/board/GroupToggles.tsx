import { toggleRect, viewOf } from '../../draw/geometry'
import { groupTitle } from '../../model/groups'
import { useStore } from '../../state/store'

/**
 * One real button over the fold arrow in each box header, so collapsing is reachable with Tab and Enter or Space as
 * well as the pointer. The arrow itself is drawn on the canvas; this button only sits on top of it.
 */
export function GroupToggles() {
  const diagram = useStore((s) => s.diagram)
  const reg = useStore((s) => s.reg)
  const vp = useStore((s) => s.viewport)
  const toggle = useStore((s) => s.toggleCollapsed)
  if (!reg || !diagram.groups?.length) return null
  const view = viewOf(diagram, reg)
  return (
    <>
      {diagram.groups.filter((g) => !view.hiddenGroups.has(g.id)).map((g) => {
        const r = toggleRect(g, view, vp.zoom)
        const w = Math.max(24, r.w * vp.zoom)
        const h = Math.max(24, r.h * vp.zoom)
        const cx = (r.x + r.w / 2) * vp.zoom + vp.scrollX
        const cy = (r.y + r.h / 2) * vp.zoom + vp.scrollY
        const name = groupTitle(g)
        const label = `${g.collapsed ? 'Expand' : 'Collapse'} ${name}`
        return (
          <button
            key={g.id}
            className="group-toggle"
            style={{ left: cx - w / 2, top: cy - h / 2, width: w, height: h }}
            aria-label={label}
            aria-expanded={!g.collapsed}
            title={label}
            onClick={() => toggle(g.id)}
          />
        )
      })}
    </>
  )
}
