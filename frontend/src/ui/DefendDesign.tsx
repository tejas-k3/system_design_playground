import { useEffect, useState } from 'react'
import { defendDesign, fetchAlternatives } from '../api/client'
import type { Alternative, DefendResponse, Diagram } from '../api/types'
import { defendQuestion, forKey, selectDefendAlternative } from '../model/defend'
import { Rich } from './Term'

export function DefendDesign({ id, diagram, seed }: { id: string; diagram: Diagram; seed?: number }) {
  // each piece of state remembers what it was made for, so a challenge or a design that has changed shows nothing stale
  const [picked, setPicked] = useState<{ key: string; value: { index: number; alternative: Alternative } | null } | null>(null)
  const [answer, setAnswer] = useState<{ key: Diagram; value: DefendResponse } | null>(null)
  const [busy, setBusy] = useState(false)
  const choice = forKey(picked, id)
  const result = forKey(answer, diagram)

  useEffect(() => {
    let live = true
    void fetchAlternatives(id).then((list) => {
      if (!live) return
      const selected = selectDefendAlternative(list)
      setPicked({ key: id, value: selected === null ? null : { index: selected, alternative: list[selected] } })
    }).catch(() => {
      if (live) setPicked({ key: id, value: null })
    })
    return () => { live = false }
  }, [id])

  if (!choice) return null
  const { alternative, index } = choice

  async function compare() {
    setBusy(true)
    try {
      setAnswer({ key: diagram, value: await defendDesign(id, diagram, index, seed) })
    } catch (e) {
      setAnswer({ key: diagram, value: { applicable: false, reason: e instanceof Error ? e.message : 'The comparison could not run.' } })
    } finally {
      setBusy(false)
    }
  }

  return (
    <section aria-label="Defend your design" className="defend">
      <h3>Defend your design</h3>
      <p>{defendQuestion(alternative.label)}</p>
      {!result && <button type="button" onClick={() => void compare()} disabled={busy}>{busy ? 'Running comparison…' : 'Try it on my design'}</button>}
      {result && !result.applicable && <p role="status" className="muted">{result.reason}</p>}
      {result?.applicable && result.yours && result.withChange && result.alternative && <>
        <div className="two-col defend-results" aria-label="Design comparison">
          <div><b>Your design</b><DefendMetrics score={result.yours} /></div>
          <div><b>With the change</b><DefendMetrics score={result.withChange} /></div>
        </div>
        <p className="muted small"><Rich text={result.alternative.why} /></p>
      </>}
    </section>
  )
}

function DefendMetrics({ score }: { score: NonNullable<DefendResponse['yours']> }) {
  return <ul className="defend-metrics">
    <li>{score.pass ? 'Passes' : 'Does not pass'}</li>
    <li>Success: {score.successPct.toFixed(1)}%</li>
    <li>p95: {score.successPct === 0 ? '—' : `${Math.round(score.p95Ms)} ms`}</li>
    <li>Cost: ${score.costPerHour.toFixed(2)}/hr</li>
  </ul>
}
