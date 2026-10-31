import { useEffect, useState } from 'react'
import { fetchScenariosOrCached } from '../api/client'
import type { ScenarioSummary } from '../api/types'
import { reset } from '../sim/bridge'
import { loadScenario } from '../sim/loadScenario'
import { loadDebrief } from '../state/learning'
import { useStore } from '../state/store'
import { starTargets } from '../model/goals'
import { Stars } from './Stars'
import { Alternatives } from './Alternatives'
import { DefendDesign } from './DefendDesign'
import { TradeOffMemo } from './TradeOffMemo'
import { WeakPointChecklist } from './WeakPointChecklist'
import { Rich } from './Term'
import { useEscapeKey } from './useEscapeKey'

export function ResultModal() {
  const sc = useStore((s) => s.scenario)
  const diagram = useStore((s) => s.diagram)
  const findings = useStore((s) => s.findings)
  const open = useStore((s) => s.ui.result)
  const setUi = useStore((s) => s.setUi)
  const [list, setList] = useState<ScenarioSummary[]>([])
  useEffect(() => {
    if (open) fetchScenariosOrCached().then((r) => setList(r.list)).catch(() => undefined)
  }, [open])
  useEffect(() => { // the debrief comes with a passing result, not with the challenge
    if (open && sc?.result?.pass && sc.debrief === null) void loadDebrief(sc.def.id)
  }, [open, sc?.result?.pass, sc?.debrief, sc?.def.id])
  useEscapeKey(open && !!sc?.result, () => setUi({ result: false }))
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
        {sc.dailySeed !== undefined && <p className="muted center small">Today's run: seed {sc.dailySeed}</p>}
        {r.stars < 3 && r.pass && <p className="muted center small">{starTargets(sc.def.goals)}</p>}
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
        {r.pass && sc.debrief && <p className="debrief"><Rich text={sc.debrief} /></p>}
        {r.pass && sc.debrief && <WeakPointChecklist key={sc.def.id} points={sc.weakPoints} />}
        {r.pass && sc.debrief && sc.deeper && <Deeper key={sc.def.id} text={sc.deeper} />}
        {r.pass && <Alternatives id={sc.def.id} />}
        {r.pass && <DefendDesign id={sc.def.id} diagram={diagram} seed={sc.dailySeed} />}
        {r.pass && <TradeOffMemo key={sc.def.id} id={sc.def.id} title={sc.def.title} score={r} findings={findings} seconds={Math.max(1, sc.def.durationSec - sc.def.warmupSec)} />}
        <div className="row-btns end">
          <button autoFocus onClick={close}>Keep tweaking</button>
          <button onClick={() => { close(); reset() }}>Try again</button>
          {r.pass && next && <button className="primary" onClick={() => { close(); loadScenario(next.id) }}>Next: {next.title}</button>}
        </div>
      </div>
    </div>
  )
}

/** The optional longer explanation: collapsed until asked for. */
function Deeper({ text }: { text: string }) {
  const [shown, setShown] = useState(false)
  return (
    <div className="deeper">
      <button aria-expanded={shown} aria-controls="deeper-text" onClick={() => setShown(!shown)}>{shown ? 'Hide the deeper explanation' : 'Go deeper'}</button>
      {shown && <p id="deeper-text" className="debrief"><Rich text={text} /></p>}
    </div>
  )
}
