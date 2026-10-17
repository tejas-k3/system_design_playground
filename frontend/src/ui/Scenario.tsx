import { useEffect, useState } from 'react'
import { fetchScenario, fetchScenarios, fetchSolution } from '../api/client'
import type { ScenarioSummary } from '../api/types'
import { goalRows } from '../model/goals'
import { exampleDiagram } from '../model/examples'
import { reset } from '../sim/bridge'
import { useStore } from '../state/store'

export const Stars = ({ n, size = 16 }: { n: number; size?: number }) => (
  <span className="stars" aria-label={`${n} of 3 stars`}>
    {[1, 2, 3].map((i) => (
      <svg key={i} width={size} height={size} viewBox="0 0 24 24" className={i <= n ? 'on' : ''} aria-hidden>
        <path d="M12 3l2.7 5.6 6.1.8-4.5 4.3 1.1 6.1L12 17l-5.4 2.8 1.1-6.1L3.2 9.4l6.1-.8z" />
      </svg>
    ))}
  </span>
)

export async function loadScenario(id: string) {
  const st = useStore.getState()
  try {
    reset()
    st.openScenario(await fetchScenario(id))
    st.requestFit()
  } catch (e) {
    st.toast(`Could not load challenge: ${(e as Error).message}`, 'error')
  }
}

export function ScenarioPanel() {
  const sc = useStore((s) => s.scenario)
  const stats = useStore((s) => s.stats)
  const cost = useStore((s) => s.cost)
  const status = useStore((s) => s.sim.status)
  const revealHint = useStore((s) => s.revealHint)
  const answer = useStore((s) => s.answerPrediction)
  const showSolution = useStore((s) => s.showSolution)
  const exit = useStore((s) => s.exitScenario)
  const [open, setOpen] = useState(true)
  useEffect(() => setOpen(true), [sc?.def.id])
  if (!sc) return null
  const { def } = sc
  const rows = goalRows(def.goals, stats, cost, status !== 'stopped')
  const pred = def.predictions?.[0]
  const picked = pred ? sc.predictions[pred.id] : undefined

  return (
    <section className={`scenario ${open ? '' : 'closed'}`} aria-label="Challenge">
      <header>
        <button className="link" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? '▾' : '▸'}</button>
        <h2>{def.title}</h2>
        <span className={`badge ${def.difficulty.toLowerCase()}`}>{def.difficulty}</span>
        {def.configOnly && <span className="badge cfg" title="The architecture is fixed; only settings can change">config only</span>}
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

export function ResultModal() {
  const sc = useStore((s) => s.scenario)
  const open = useStore((s) => s.ui.result)
  const setUi = useStore((s) => s.setUi)
  const [list, setList] = useState<ScenarioSummary[]>([])
  useEffect(() => {
    if (open) fetchScenarios().then(setList).catch(() => undefined)
  }, [open])
  if (!sc?.result || !open) return null
  const r = sc.result
  const idx = list.findIndex((s) => s.id === sc.def.id)
  const next = idx >= 0 ? list[idx + 1] : undefined
  const close = () => setUi({ result: false })
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Result" onClick={close}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>{r.pass ? 'Challenge complete!' : 'Not quite yet'}</h2>
        <div className="center"><Stars n={r.stars} size={34} /></div>
        <p className="muted center small">
          {r.stars === 3 ? 'Fast and cheap: all three stars.' : r.stars === 2 ? 'Passing and affordable. A faster design earns the third star.' : r.stars === 1 ? 'Passing, but over the star budget. Cut cost to earn another star.' : 'Some goals failed. Read the table below, then adjust your design.'}
        </p>
        <table className="checks">
          <tbody>
            {r.checks.map((c) => (
              <tr key={c.label} className={c.pass ? 'pass' : 'fail'}>
                <td>{c.pass ? '✓' : '✕'}</td>
                <td>{c.label}</td>
                <td>{c.target}</td>
                <td><b>{c.actual}</b></td>
              </tr>
            ))}
          </tbody>
        </table>
        {r.pass && <p className="debrief">{sc.def.debrief}</p>}
        <div className="row-btns end">
          <button onClick={close}>Keep tweaking</button>
          <button onClick={() => { close(); reset() }}>Try again</button>
          {r.pass && next && <button className="primary" onClick={() => { close(); loadScenario(next.id) }}>Next: {next.title}</button>}
        </div>
      </div>
    </div>
  )
}

export function ScenarioPicker() {
  const open = useStore((s) => s.ui.picker)
  const setUi = useStore((s) => s.setUi)
  const progress = useStore((s) => s.progress)
  const active = useStore((s) => s.scenario?.def.id)
  const exit = useStore((s) => s.exitScenario)
  const [list, setList] = useState<ScenarioSummary[] | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    if (!open) return
    fetchScenarios().then(setList).catch((e: Error) => setErr(e.message))
  }, [open])
  if (!open) return null
  const groups = ['Tutorial', 'Easy', 'Medium', 'Hard']
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Challenges" onClick={() => setUi({ picker: false })}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head">
          <h2>Challenges</h2>
          <button className="x" onClick={() => setUi({ picker: false })} aria-label="Close">×</button>
        </header>
        <p className="muted small">Each challenge starts with a broken system. Change it until it meets the goals. Earn up to three stars.</p>
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
                    {s.configOnly && <em>config only</em>}
                  </button>
                ))}
              </div>
            </section>
          )
        })}
        <div className="row-btns end">
          <button onClick={() => { reset(); exit(); setUi({ picker: false }) }}>{active ? 'Back to sandbox' : 'Close'}</button>
        </div>
      </div>
    </div>
  )
}

export function Welcome() {
  const show = useStore((s) => s.ui.welcome && !s.scenario && s.diagram.nodes.length === 0)
  const loadDiagram = useStore((s) => s.loadDiagram)
  const setUi = useStore((s) => s.setUi)
  const requestFit = useStore((s) => s.requestFit)
  if (!show) return null
  return (
    <div className="welcome">
      <h1>Sketch a system. Send traffic. Watch it break.</h1>
      <p>Drag components from the left onto the canvas, wire them together, then press Play.</p>
      <div className="row-btns center">
        <button className="primary" onClick={() => { loadDiagram(exampleDiagram()); setUi({ welcome: false }); requestFit() }}>Load an example</button>
        <button onClick={() => setUi({ picker: true })}>Start a challenge</button>
        <button onClick={() => setUi({ welcome: false })}>Start blank</button>
      </div>
    </div>
  )
}

export function Help() {
  const open = useStore((s) => s.ui.help)
  const setUi = useStore((s) => s.setUi)
  if (!open) return null
  const keys: [string, string][] = [
    ['Space', 'Play / pause'],
    ['V · C', 'Select tool · Connect tool'],
    ['Delete', 'Delete selection'],
    ['Ctrl/⌘ Z · Shift+Z', 'Undo · Redo'],
    ['F', 'Fit the design on screen'],
    ['+ / −  ·  scroll', 'Zoom'],
    ['Drag empty space', 'Pan the canvas'],
    ['Shift + drag', 'Box-select'],
    ['Double-click empty', 'Add a note'],
    ['Esc', 'Clear selection / cancel connecting'],
  ]
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Help" onClick={() => setUi({ help: false })}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head"><h2>How it works</h2><button className="x" onClick={() => setUi({ help: false })} aria-label="Close">×</button></header>
        <ol className="how">
          <li><b>Build.</b> Add users, servers, caches, databases, queues… and draw connections (drag from the round handle on a component's right edge).</li>
          <li><b>Run.</b> Press Play. Requests flow along the connections; bars show how busy each part is and why requests fail.</li>
          <li><b>Break it.</b> Select a component and press "Crash it", or raise the traffic, and watch what gives way first.</li>
          <li><b>Fix it.</b> Change settings or structure live. The simulation keeps running while you edit.</li>
        </ol>
        <h3>Shortcuts</h3>
        <dl className="kv keys">{keys.map(([k, v]) => (<><dt key={k}><kbd>{k}</kbd></dt><dd key={v}>{v}</dd></>))}</dl>
        <p className="muted small">Runs are deterministic: the same design and seed always produce the same result.</p>
      </div>
    </div>
  )
}

export function Toasts() {
  const toasts = useStore((s) => s.toasts)
  const dismiss = useStore((s) => s.dismissToast)
  return (
    <div className="toasts" aria-live="polite">
      {toasts.map((t) => (
        <div key={t.id} className={`toast ${t.kind}`} onClick={() => dismiss(t.id)} role="status">{t.text}</div>
      ))}
    </div>
  )
}
