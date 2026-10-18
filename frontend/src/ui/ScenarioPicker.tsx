import { useEffect, useState } from 'react'
import { fetchQuestions, fetchScenarios } from '../api/client'
import type { QuestionSummary, ScenarioSummary } from '../api/types'
import { reset } from '../sim/bridge'
import { loadScenario } from '../sim/loadScenario'
import { useStore } from '../state/store'
import { CONFIG_ONLY_HELP } from './copy'
import { Stars } from './Stars'
import { useEscapeKey } from './useEscapeKey'

export function ScenarioPicker() {
  const open = useStore((s) => s.ui.picker)
  const setUi = useStore((s) => s.setUi)
  const progress = useStore((s) => s.progress)
  const active = useStore((s) => s.scenario?.def.id)
  const exit = useStore((s) => s.exitScenario)
  const [list, setList] = useState<ScenarioSummary[] | null>(null)
  const [questions, setQuestions] = useState<QuestionSummary[]>([])
  const [err, setErr] = useState('')
  useEffect(() => {
    if (!open) return
    fetchScenarios().then(setList).catch((e: Error) => setErr(e.message))
    fetchQuestions().then(setQuestions).catch(() => setQuestions([]))
  }, [open])
  useEscapeKey(open, () => setUi({ picker: false }))
  if (!open) return null
  const groups = ['Tutorial', 'Easy', 'Medium', 'Hard']
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Challenges" onClick={() => setUi({ picker: false })}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head">
          <h2>Challenges</h2>
          <button className="x" autoFocus onClick={() => setUi({ picker: false })} aria-label="Close">×</button>
        </header>
        <p className="muted small">Each challenge starts with a broken system. Change it until it meets the goals. Earn up to three stars. <em>config only</em> means {CONFIG_ONLY_HELP.charAt(0).toLowerCase() + CONFIG_ONLY_HELP.slice(1)}</p>
        {err && <p className="bad">{err}</p>}
        {!list && !err && <p className="muted">Loading…</p>}
        {groups.map((g) => {
          const items = list?.filter((s) => s.difficulty === g) ?? []
          if (!items.length) return null
          return (
            <section key={g}>
              <h3>{g}</h3>
              <div className="cards">
                {items.map((s) => (
                  <button key={s.id} className={`card ${active === s.id ? 'active' : ''}`} onClick={() => loadScenario(s.id)}>
                    <span className="card-top"><b>{s.title}</b><Stars n={progress[s.id] ?? 0} size={14} /></span>
                    <small>{s.summary}</small>
                    {s.configOnly && <em title={CONFIG_ONLY_HELP}>config only</em>}
                    {s.open && <em title="Blank canvas, hidden traffic, no solution to reveal">open</em>}
                  </button>
                ))}
              </div>
            </section>
          )
        })}
        {questions.length > 0 && (
          <section aria-label="Questions">
            <h3>Questions</h3>
            <p className="muted small">No design to fix: answer, then see how the numbers work out.</p>
            <div className="cards">
              {questions.map((q) => (
                <button key={q.id} className="card" onClick={() => setUi({ picker: false, question: q.id })}>
                  <span className="card-top"><b>{q.title}</b><Stars n={progress[q.id] ?? 0} size={14} /></span>
                  <small>{q.summary}</small>
                  <em>{q.mode === 'tradeoff' ? 'trade-off' : q.mode}</em>
                </button>
              ))}
            </div>
          </section>
        )}
        <div className="row-btns end">
          <button onClick={() => { reset(); exit(); setUi({ picker: false }) }}>{active ? 'Back to sandbox' : 'Close'}</button>
        </div>
      </div>
    </div>
  )
}
