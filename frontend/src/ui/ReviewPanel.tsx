import { groupFindings, severityLabel, summarise } from '../model/lint'
import { useStore } from '../state/store'

/** Advisory design review: lint findings grouped by severity, each able to select the components it names. */
export function ReviewPanel() {
  const findings = useStore((s) => s.findings)
  const nodes = useStore((s) => s.diagram.nodes)
  const setSelection = useStore((s) => s.setSelection)
  if (nodes.length === 0) return null
  const name = (id: string) => nodes.find((n) => n.id === id)?.label ?? id
  return (
    <section className="review" aria-label="Design review">
      <h3>Design review</h3>
      <p className="muted small" role="status">{summarise(findings)}</p>
      {groupFindings(findings).map((g) => (
        <ul key={g.severity} className={`findings ${g.severity}`} aria-label={`${severityLabel(g.severity)} findings`}>
          {g.items.map((f, i) => (
            <li key={f.rule + i}>
              <span className="sev">{severityLabel(f.severity)}</span> {f.message}
              <div className="muted small">Fix: {f.fix}</div>
              <button className="link" onClick={() => setSelection(f.nodes)}>
                Select {f.nodes.length === 1 ? name(f.nodes[0]) : `${f.nodes.length} components`}
              </button>
            </li>
          ))}
        </ul>
      ))}
    </section>
  )
}
