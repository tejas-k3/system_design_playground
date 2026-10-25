import { useEffect, useMemo, useState } from 'react'
import { viewOf } from '../draw/geometry'
import { groupTitle } from '../model/groups'
import { searchNodes } from '../model/search'
import { useStore } from '../state/store'
import { useEscapeKey } from './useEscapeKey'

/** Ctrl/Cmd+K: find a component by name or type; Enter selects it and brings it to the middle of the view. */
export function SearchDialog() {
  const open = useStore((s) => s.ui.search)
  const setUi = useStore((s) => s.setUi)
  const diagram = useStore((s) => s.diagram)
  const reg = useStore((s) => s.reg)
  const focusNode = useStore((s) => s.focusNode)
  const [query, setQuery] = useState('')
  const [at, setAt] = useState(0)
  const close = () => setUi({ search: false })
  useEscapeKey(open, close)
  const found = useMemo(() => searchNodes(diagram, (k) => reg?.spec(k)?.name ?? k, query), [diagram, reg, query])
  useEffect(() => setAt(0), [query])
  useEffect(() => {
    if (open) setQuery('')
  }, [open])
  if (!open) return null
  const view = reg ? viewOf(diagram, reg) : null
  const boxOf = (id: string) => diagram.groups?.find((g) => g.id === view?.owner.get(id))
  const pick = (i: number) => {
    const n = found[i]
    if (!n) return
    focusNode(n.id)
    close()
  }
  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') (e.preventDefault(), setAt((a) => Math.min(found.length - 1, a + 1)))
    else if (e.key === 'ArrowUp') (e.preventDefault(), setAt((a) => Math.max(0, a - 1)))
    else if (e.key === 'Enter') (e.preventDefault(), pick(at))
  }
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Find a component" onClick={close}>
      <div className="modal search" onClick={(e) => e.stopPropagation()}>
        <input
          autoFocus
          type="search"
          placeholder="Find a component by name or type"
          aria-label="Find a component by name or type"
          aria-controls="search-results"
          aria-activedescendant={found[at] ? `search-${found[at].id}` : undefined}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKey}
        />
        <ul id="search-results" role="listbox" aria-label="Matching components">
          {found.map((n, i) => (
            <li key={n.id} id={`search-${n.id}`} role="option" aria-selected={i === at} className={i === at ? 'on' : ''} onMouseEnter={() => setAt(i)} onClick={() => pick(i)}>
              <span>{n.label}</span> <span className="muted small">{reg?.spec(n.kind)?.name ?? n.kind}{boxOf(n.id) ? ` · inside collapsed ${groupTitle(boxOf(n.id)!)}, opens it` : ''}</span>
            </li>
          ))}
        </ul>
        {found.length === 0 && <p className="muted small" role="status">Nothing matches.</p>}
      </div>
    </div>
  )
}
