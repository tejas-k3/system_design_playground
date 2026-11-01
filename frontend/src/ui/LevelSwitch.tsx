import { LEARNER_LEVELS, studyDeckUrl } from '../model/level'
import { useStore } from '../state/store'

/** Pick your level: SDE1, SDE2, SDE3 or all. The default, all levels, hides nothing; a level only changes the order. */
export function LevelSwitch() {
  const level = useStore((s) => s.level)
  const setLevel = useStore((s) => s.setLevel)
  const deck = studyDeckUrl(level)
  return (
    <div className="level-switch">
      <span className="muted small" id="level-label">Your level</span>
      <div className="seg" role="group" aria-labelledby="level-label">
        {LEARNER_LEVELS.map((l) => (
          <button key={l.id} aria-pressed={level === l.id} className={level === l.id ? 'on' : ''} onClick={() => setLevel(l.id)}>{l.label}</button>
        ))}
      </div>
      {deck && <a className="small" href={deck} target="_blank" rel="noopener noreferrer" title="Opens a text page with the answers: counter-questions with measured evidence, for a study partner">Study deck</a>}
    </div>
  )
}
