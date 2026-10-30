import { useEffect, useState } from 'react'
import { defendDesign, fetchAlternatives } from '../api/client'
import type { Alternative, DefendResponse, Diagram } from '../api/types'
import { defendQuestion, selectDefendAlternative } from '../model/defend'
import { Rich } from './Term'

export function DefendDesign({ id, diagram, seed }: { id: string; diagram: Diagram; seed?: number }) {
  const [alternative, setAlternative] = useState<Alternative | null>(null)
  const [index, setIndex] = useState<number | null>(null)
  const [result, setResult] = useState<DefendResponse | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let live = true
    setResult(null)
    void fetchAlternatives(id).then((list) => {
      if (!live) return
      const selected = selectDefendAlternative(list)
      setIndex(selected)
      setAlternative(selected === null ? null : list[selected])
    }).catch(() => {
      if (live) setAlternative(null)
    })
    return () => { live = false }
  }, [id])

  if (!alternative || index === null) return null

  async function compare() {
    if (index === null) return
    setBusy(true)
    try {
      setResult(await defendDesign(id, diagram, index, seed))
    } catch (e) {
      setResult({ applicable: false, reason: e instanceof Error ? e.message : 'The comparison could not run.' })
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
