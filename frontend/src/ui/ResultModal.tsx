import { useEffect, useState } from 'react'
import { fetchScenarios } from '../api/client'
import type { ScenarioSummary } from '../api/types'
import { reset } from '../sim/bridge'
import { loadScenario } from '../sim/loadScenario'
import { useStore } from '../state/store'
import { Stars } from './Stars'
import { useEscapeKey } from './useEscapeKey'

export function ResultModal() {
  const sc = useStore((s) => s.scenario)
  const open = useStore((s) => s.ui.result)
  const setUi = useStore((s) => s.setUi)
  const [list, setList] = useState<ScenarioSummary[]>([])
  useEffect(() => {
    if (open) fetchScenarios().then(setList).catch(() => undefined)
  }, [open])
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
          <button autoFocus onClick={close}>Keep tweaking</button>
          <button onClick={() => { close(); reset() }}>Try again</button>
          {r.pass && next && <button className="primary" onClick={() => { close(); loadScenario(next.id) }}>Next: {next.title}</button>}
        </div>
      </div>
    </div>
  )
}
