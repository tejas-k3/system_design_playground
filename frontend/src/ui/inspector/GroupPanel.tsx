import type { GroupKind } from '../../api/types'
import { nodeRect } from '../../draw/geometry'
import { GROUP_KINDS, MAX_GROUP, MAX_GROUP_LABEL, MIN_GROUP, groupInfo, groupMembers } from '../../model/groups'
import { useStore } from '../../state/store'

export function GroupPanel({ id }: { id: string }) {
  const group = useStore((s) => s.diagram.groups?.find((g) => g.id === id))
  const diagram = useStore((s) => s.diagram)
  const reg = useStore((s) => s.reg)
  const updateGroup = useStore((s) => s.updateGroup)
  const del = useStore((s) => s.deleteIds)
  if (!group || !reg) return null
  const inside = groupMembers(diagram, id, (n) => nodeRect(n, reg))
  const comps = inside.filter((i) => diagram.nodes.some((n) => n.id === i)).length
  const other = inside.length - comps
  return (
    <div className="panel">
      <h2>{groupInfo(group.kind).name} box</h2>
      <div className="field">
        <label htmlFor="gl">Name</label>
        <input id="gl" type="text" value={group.label} maxLength={MAX_GROUP_LABEL} onChange={(e) => updateGroup(id, { label: e.target.value })} />
      </div>
      <div className="field">
        <label htmlFor="gk">Kind</label>
        <select id="gk" value={group.kind} onChange={(e) => updateGroup(id, { kind: e.target.value as GroupKind })}>
          {GROUP_KINDS.map((k) => (<option key={k.kind} value={k.kind}>{k.name}</option>))}
        </select>
      </div>
      <div className="field">
        <label htmlFor="gw">Width</label>
        <input id="gw" type="number" min={MIN_GROUP} max={MAX_GROUP} step={20} value={Math.round(group.w)} onChange={(e) => Number.isFinite(e.target.valueAsNumber) && updateGroup(id, { w: e.target.valueAsNumber })} />
      </div>
      <div className="field">
        <label htmlFor="gh">Height</label>
        <input id="gh" type="number" min={MIN_GROUP} max={MAX_GROUP} step={20} value={Math.round(group.h)} onChange={(e) => Number.isFinite(e.target.valueAsNumber) && updateGroup(id, { h: e.target.valueAsNumber })} />
      </div>
      <p className="muted small">Holds {comps} component{comps === 1 ? '' : 's'}{other > 0 ? ` and ${other} other item${other === 1 ? '' : 's'}` : ''}. Moving the box moves them. The simulation ignores boxes.</p>
      <button className="danger-outline" onClick={() => del([id])}>Delete box (keeps what is inside)</button>
    </div>
  )
}
