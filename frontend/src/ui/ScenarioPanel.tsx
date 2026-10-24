import { useEffect, useState } from 'react'
import { answerPrediction, fetchHint, fetchSolution } from '../api/client'
import { describeDiff, diffDiagrams } from '../model/diff'
import { goalCost, goalRows, starTargets } from '../model/goals'
import { isNarrow } from '../model/responsive'
import { reset } from '../sim/bridge'
import { loadAnswer } from '../state/learning'
import { useStore } from '../state/store'
import { CONFIG_ONLY_HELP } from './copy'
import { Rich } from './Term'

export function ScenarioPanel() {
  const sc = useStore((s) => s.scenario)
  const stats = useStore((s) => s.stats)
  const cost = useStore((s) => s.cost)
  const status = useStore((s) => s.sim.status)
  const ui = useStore((s) => s.ui)
  const setUi = useStore((s) => s.setUi)
  const addHint = useStore((s) => s.addHint)
  const recordPrediction = useStore((s) => s.recordPrediction)
  const [asking, setAsking] = useState(false) // a hint or an answer is on its way from the server
  const showSolution = useStore((s) => s.showSolution)
  const exit = useStore((s) => s.exitScenario)
  const compareWith = useStore((s) => s.compareWithReference)
  const reg = useStore((s) => s.reg)
  const [open, setOpen] = useState(!isNarrow()) // on a phone the story starts folded away so the board is visible
  useEffect(() => setOpen(!isNarrow()), [sc?.def.id])
  if (!sc) return null
  const { def } = sc
  const rows = goalRows(def.goals, stats, goalCost(cost, stats, status !== 'stopped'), status !== 'stopped')
  const stars = starTargets(def.goals)
  const pred = def.predictions?.[0]
  const picked = pred ? sc.predictions[pred.id] : undefined // what the server said to the player's choice

  return (
    <section className={`scenario ${open ? '' : 'closed'}`} aria-label="Challenge">
      <header>
        <button className="link" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? '▾' : '▸'}</button>
        <h2>{def.title}</h2>
        <span className={`badge ${def.difficulty.toLowerCase()}`}>{def.difficulty}</span>
        {def.configOnly && <span className="badge cfg" title={CONFIG_ONLY_HELP}>config only</span>}
        {def.open && <span className="badge cfg" title="Start from a blank canvas. The traffic is not shown and there is no reference solution; the requirements are the test.">open</span>}
        <button className="x" onClick={() => { reset(); exit() }} aria-label="Leave challenge" title="Leave challenge">×</button>
      </header>
      {open && (
        <div className="scenario-body">
          <p><Rich text={def.story} /></p>
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
          {sc.result && !ui.result && (
            <p className="result-again">
              <button onClick={() => setUi({ result: true })}>Show the result{sc.result.pass ? ' (passed)' : ''}</button>
              <span className="muted small"> {sc.result.pass ? '' : 'Change the design, then press Run again.'}</span>
            </p>
          )}
          {!sc.result && status !== 'stopped' && rows.length > 0 && rows.every((r) => r.state === 'pass') && (
            <p className="muted small" role="status">All goals are met so far. The final result appears when the run ends{status === 'paused' ? ': press Resume to let it finish.' : '.'}</p>
          )}
          {pred && status === 'stopped' && (
            <div className="predict">
              <h3>Predict first</h3>
              <p>{pred.question}</p>
              <div className="choices">
                {pred.options.map((o, i) => (
                  <button key={o} disabled={picked !== undefined || asking} className={picked === undefined ? '' : i === picked.answer ? 'right' : i === picked.choice ? 'wrong' : ''} onClick={async () => {
                    setAsking(true)
                    try {
                      recordPrediction(pred, await answerPrediction(def.id, pred.id, i))
                    } catch (e) {
                      useStore.getState().toast((e as Error).message, 'error')
                    } finally {
                      setAsking(false)
                    }
                  }}>
                    {o}
                  </button>
                ))}
              </div>
              {picked !== undefined && <p className="muted small">{picked.explain}</p>}
            </div>
          )}
          {sc.hintLevel > 0 && (
            <ol className="hints">
              {sc.hints.map((h, i) => <li key={i}><Rich text={h} /></li>)}
            </ol>
          )}
          {sc.answer && <p className="answer" role="note"><b>The answer:</b> <Rich text={sc.answer} /></p>}
          {sc.result?.pass && !sc.compare && !def.open && (
            <p>
              <button onClick={async () => {
                try {
                  compareWith(await fetchSolution(def.id))
                } catch (e) {
                  useStore.getState().toast((e as Error).message, 'error')
                }
              }}>Compare with the reference</button>
            </p>
          )}
          {sc.compare && reg && <VsReference mine={sc.compare.mine} reference={sc.compare.reference} />}
          <div className="row-btns">
            <button disabled={sc.hintLevel >= def.hintCount || asking} onClick={async () => {
              setAsking(true)
              try {
                addHint((await fetchHint(def.id, sc.hintLevel + 1)).hint)
              } catch (e) {
                useStore.getState().toast((e as Error).message, 'error')
              } finally {
                setAsking(false)
              }
            }}>{sc.hintLevel === 0 ? 'Give me a hint' : sc.hintLevel < def.hintCount ? 'Another hint' : 'No more hints'}</button>
            {sc.hintLevel >= def.hintCount && !sc.answer && <button
              disabled={asking}
              title="Show the full answer. The challenge will be marked for review on the skill map."
              onClick={async () => {
                setAsking(true)
                try {
                  await loadAnswer(def.id)
                } finally {
                  setAsking(false)
                }
              }}
            >
              Show the answer
            </button>}
            {!def.open && <button
              disabled={sc.hintLevel < def.hintCount}
              title={sc.hintLevel < def.hintCount ? 'Use all three hints first' : 'Replace your design with the reference solution'}
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
            </button>}
          </div>
        </div>
      )}
    </section>
  )
}

function VsReference({ mine, reference }: { mine: import('../api/types').Diagram; reference: import('../api/types').Diagram }) {
  const reg = useStore((s) => s.reg)!
  const diff = diffDiagrams(mine, reference, reg)
  const lines = describeDiff(diff, reg)
  return (
    <section className="vs-reference" aria-label="Your design versus the reference">
      <h3>Your design vs the reference</h3>
      {lines.length === 0 ? <p className="muted small">Identical to the reference.</p> : (
        <>
          <p className="muted small">{diff.sameArchitecture ? 'Same components and connections as the reference; only settings differ.' : `${lines.length} ${lines.length === 1 ? 'difference' : 'differences'}. Other designs can work too: this is the reference's way.`}</p>
          <ul>{lines.map((l, i) => <li key={i}>{l}</li>)}</ul>
        </>
      )}
    </section>
  )
}
