import { templateDiagram, templates } from '../model/templates'
import { useStore } from '../state/store'
import { useEscapeKey } from './useEscapeKey'

/** A gallery of starter designs; choosing one replaces the canvas in a single undoable step. */
export function TemplatesDialog() {
  const open = useStore((s) => s.ui.templates)
  const setUi = useStore((s) => s.setUi)
  const applyTemplate = useStore((s) => s.applyTemplate)
  const hasWork = useStore((s) => s.diagram.nodes.length > 0)
  const close = () => setUi({ templates: false })
  useEscapeKey(open, close)
  if (!open) return null
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Starter templates" onClick={close}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head"><h2>Starter templates</h2><button className="x" autoFocus onClick={close} aria-label="Close">×</button></header>
        <p className="muted small">{hasWork ? 'Choosing one replaces what is on the canvas; Undo brings it back.' : 'Pick a design to start from, then change it.'}</p>
        <ul className="templates">
          {templates.map((t) => (
            <li key={t.id}>
              <button onClick={() => { const d = templateDiagram(t.id); if (d) applyTemplate(d) }}>
                <b>{t.title}</b>
                <span className="muted small">{t.summary}</span>
                <span className="muted small">{t.diagram.nodes.map((n) => n.label).join(' → ')}</span>
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
