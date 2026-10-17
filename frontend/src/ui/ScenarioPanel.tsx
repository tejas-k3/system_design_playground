import { useEffect, useState } from 'react'
import { fetchSolution } from '../api/client'
import { goalRows, starTargets } from '../model/goals'
import { isNarrow } from '../model/responsive'
import { reset } from '../sim/bridge'
import { useStore } from '../state/store'
import { CONFIG_ONLY_HELP } from './copy'

export function ScenarioPanel() {
  const sc = useStore((s) => s.scenario)
  const stats = useStore((s) => s.stats)
  const cost = useStore((s) => s.cost)
  const status = useStore((s) => s.sim.status)
  const revealHint = useStore((s) => s.revealHint)
  const answer = useStore((s) => s.answerPrediction)
  const showSolution = useStore((s) => s.showSolution)
  const exit = useStore((s) => s.exitScenario)
  const [open, setOpen] = useState(!isNarrow()) // on a phone the story starts folded away so the board is visible
  useEffect(() => setOpen(!isNarrow()), [sc?.def.id])
  if (!sc) return null
  const { def } = sc
  const rows = goalRows(def.goals, stats, cost, status !== 'stopped')
  const stars = starTargets(def.goals)
  const pred = def.predictions?.[0]
  const picked = pred ? sc.predictions[pred.id] : undefined

  return (
    <section className={`scenario ${open ? '' : 'closed'}`} aria-label="Challenge">
      <header>
        <button className="link" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? '▾' : '▸'}</button>
        <h2>{def.title}</h2>
        <span className={`badge ${def.difficulty.toLowerCase()}`}>{def.difficulty}</span>
        {def.configOnly && <span className="badge cfg" title={CONFIG_ONLY_HELP}>config only</span>}
        <button className="x" onClick={() => { reset(); exit() }} aria-label="Leave challenge" title="Leave challenge">×</button>
      </header>
      {open && (
        <div className="scenario-body">
          <p>{def.story}</p>
          <h3>Goals</h3>
          <ul className="goals">
            {rows.map((r) => (
              <li key={r.label} className={r.state}>
                <span className="mark" aria-hidden>{r.state === 'pass' ? '✓' : r.state === 'fail' ? '✕' : '○'}</span>
                <span>{r.label}</span>
                <em>{r.target}</em>
                <b>{r.actual}</b>
              </li>
            ))}
          </ul>
          {stars && <p className="muted small">{stars}</p>}
          {pred && status === 'stopped' && (
            <div className="predict">
              <h3>Predict first</h3>
              <p>{pred.question}</p>
              <div className="choices">
                {pred.options.map((o, i) => (
                  <button key={o} disabled={picked !== undefined} className={picked === undefined ? '' : i === pred.answer ? 'right' : i === picked ? 'wrong' : ''} onClick={() => answer(pred, i)}>
                    {o}
                  </button>
                ))}
              </div>
              {picked !== undefined && <p className="muted small">{pred.explain}</p>}
            </div>
          )}
          {sc.hintLevel > 0 && (
            <ol className="hints">
              {def.hints.slice(0, sc.hintLevel).map((h, i) => <li key={i}>{h}</li>)}
            </ol>
          )}
          <div className="row-btns">
            <button onClick={revealHint} disabled={sc.hintLevel >= 3}>{sc.hintLevel === 0 ? 'Give me a hint' : sc.hintLevel < 3 ? 'Another hint' : 'No more hints'}</button>
            <button
              disabled={sc.hintLevel < 3}
              title={sc.hintLevel < 3 ? 'Use all three hints first' : 'Replace your design with the reference solution'}
              onClick={async () => {
                try {
                  reset()
                  showSolution(await fetchSolution(def.id))
                } catch (e) {
                  useStore.getState().toast((e as Error).message, 'error')
                }
              }}
            >
              Show solution
            </button>
          </div>
        </div>
      )}
    </section>
  )
}
