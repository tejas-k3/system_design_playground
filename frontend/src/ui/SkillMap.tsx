import { useEffect, useState } from 'react'
import { fetchScenarios } from '../api/client'
import type { ScenarioSummary } from '../api/types'
import { skillMap } from '../model/skills'
import { loadScenario } from '../sim/loadScenario'
import { useStore } from '../state/store'
import { useEscapeKey } from './useEscapeKey'

/** One row for each idea the challenges practise: how many stars you have there and what to try next. */
export function SkillMap() {
  const open = useStore((s) => s.ui.skills)
  const setUi = useStore((s) => s.setUi)
  const progress = useStore((s) => s.progress)
  const [list, setList] = useState<ScenarioSummary[] | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    if (open) fetchScenarios().then(setList).catch((e: Error) => setErr(e.message))
  }, [open])
  const close = () => setUi({ skills: false })
  useEscapeKey(open, close)
  if (!open) return null
  const rows = list ? skillMap(list, progress) : []
  const title = (id: string) => list?.find((s) => s.id === id)?.title ?? id
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Skill map" onClick={close}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head"><h2>Skill map</h2><button className="x" autoFocus onClick={close} aria-label="Close">×</button></header>
        <p className="muted small">Each row is an idea the challenges practise. The bar shows stars earned out of the stars available there; the button starts the one to try next (fewest stars first, easiest first).</p>
        {err && <p className="bad">{err}</p>}
        {!list && !err && <p className="muted">Loading…</p>}
        <ul className="skills">
          {rows.map((r) => (
            <li key={r.concept}>
              <b>{r.label}</b>
              <span className="muted small">{r.solved} of {r.total} challenges solved · {r.stars} of {r.maxStars} stars</span>
              <progress max={r.maxStars} value={r.stars} aria-label={`${r.label}: ${r.stars} of ${r.maxStars} stars`} />
              {r.next ? <button onClick={() => { setUi({ skills: false, picker: false }); loadScenario(r.next!) }}>Next: {title(r.next)}</button> : <span className="muted small">All at three stars</span>}
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
