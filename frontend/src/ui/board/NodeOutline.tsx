import { groupTitle } from '../../model/groups'
import { shapeTitle } from '../../model/shapes'
import { useStore } from '../../state/store'

/** "Cache" is already clear; "Frontend cache" is worth saying is a Cache. */
const describe = (label: string, kindName: string) => (label.startsWith(kindName) ? label : `${label}, ${kindName}`)

/**
 * A plain list of the components on the canvas, so people who cannot use a pointer (keyboard or
 * screen-reader users) can select them with Tab and Enter. Invisible until it receives focus.
 */
export function NodeOutline() {
  const diagram = useStore((s) => s.diagram)
  const reg = useStore((s) => s.reg)
  const selection = useStore((s) => s.selection)
  const setSelection = useStore((s) => s.setSelection)
  const groups = diagram.groups ?? []
  const shapes = diagram.shapes ?? []
  if (!reg || (diagram.nodes.length === 0 && groups.length === 0 && shapes.length === 0)) return null
  const label = (id: string) => diagram.nodes.find((n) => n.id === id)?.label ?? id
  return (
    <ul className="outline" aria-label="Components on the canvas">
      {groups.map((g) => (
        <li key={g.id}>
          <button aria-pressed={selection.includes(g.id)} onClick={() => setSelection([g.id])}>
            {`Box, ${groupTitle(g)}`}
          </button>
        </li>
      ))}
      {shapes.map((s) => (
        <li key={s.id}>
          <button aria-pressed={selection.includes(s.id)} onClick={() => setSelection([s.id])}>
            {`Annotation, ${shapeTitle(s)}`}
          </button>
        </li>
      ))}
      {diagram.nodes.map((n) => {
        const to = diagram.edges.filter((e) => e.from === n.id).map((e) => label(e.to))
        return (
          <li key={n.id}>
            <button aria-pressed={selection.includes(n.id)} onClick={() => setSelection([n.id])}>
              {describe(n.label, reg.spec(n.kind)?.name ?? n.kind)}
              {to.length > 0 ? `, sends to ${to.join(', ')}` : ', sends to nothing'}
            </button>
          </li>
        )
      })}
    </ul>
  )
}
