import { checklistProgress, type StarterStep } from '../model/checklist'
import { useStore } from '../state/store'

/** A guided brief's checklist: each line is a need, ticked as the matching component is placed and connected. */
export function StarterChecklist({ steps }: { steps: StarterStep[] }) {
  const diagram = useStore((s) => s.diagram)
  const progress = checklistProgress(diagram, steps)
  const done = progress.every((s) => s.done)
  return (
    <section className="starter" aria-label="Starter checklist">
      <h3>What any design like this needs</h3>
      <ul className="goals">
        {progress.map((s) => (
          <li key={s.need} className={s.done ? 'pass' : ''}>
            <span className="mark" aria-hidden>{s.done ? '✓' : '○'}</span>
            <span>{s.need}<span className="sr-only">{s.done ? ' (placed)' : ' (not placed yet)'}</span></span>
            {!s.done && <details className="small"><summary>Stuck?</summary>{s.nudge}</details>}
          </li>
        ))}
      </ul>
      <p className="muted small" role="status">{done ? 'Every part is in place and connected. Now size them: run it and read the goals.' : 'Place each one and connect it to the one above it.'}</p>
    </section>
  )
}
