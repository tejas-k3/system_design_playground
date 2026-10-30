import { useState } from 'react'
import { answerAsk } from '../api/client'
import type { AskResult } from '../api/types'
import { useStore } from '../state/store'
import { Stars } from './Stars'

interface Props {
  id: string
  asks: { text: string }[]
  picks: number
  onDone: () => void
}

/** Choose up to a few clarifying questions to ask; afterwards each one says whether it changed the design, and why. */
export function AskQuestion({ id, asks, picks, onDone }: Props) {
  const recordStars = useStore((s) => s.recordStars)
  const [chosen, setChosen] = useState<number[]>([])
  const [result, setResult] = useState<AskResult | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const full = chosen.length >= picks
  const toggle = (i: number) => setChosen(chosen.includes(i) ? chosen.filter((c) => c !== i) : full ? chosen : [...chosen, i])
  const submit = async () => {
    if (chosen.length === 0) return setError('Pick at least one question to ask.')
    setBusy(true)
    setError('')
    try {
      const r = await answerAsk(id, chosen)
      setResult(r)
      recordStars(id, r.stars)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not grade that.')
    } finally {
      setBusy(false)
    }
  }
  const again = () => {
    setChosen([])
    setResult(null)
    setError('')
  }
  return (
    <form onSubmit={(e) => { e.preventDefault(); void submit() }}>
      <fieldset disabled={!!result}>
        <legend>Questions you could ask (up to {picks})</legend>
        {asks.map((a, i) => {
          const r = result?.items[i]
          const tag = r?.decisive ? (r.chosen ? ' (changes the design: you asked it)' : ' (changes the design: you missed it)') : r?.chosen ? ' (you asked it: it changes nothing)' : ''
          return (
            <div key={i} className={`option ${r?.decisive ? 'right' : ''}`}>
              <label>
                <input type="checkbox" checked={chosen.includes(i)} disabled={!result && full && !chosen.includes(i)} onChange={() => toggle(i)} /> {a.text}
                {r && <b>{tag}</b>}
              </label>
              {r && <p className="small">{r.explain}{r.flip && <> <em>{r.flip}</em></>}</p>}
            </div>
          )
        })}
      </fieldset>
      <p className="muted small" role="status">{result ? '' : `${chosen.length} of ${picks} chosen`}</p>
      {error && <p className="bad small" role="alert">{error}</p>}
      {!result && <button className="primary" type="submit" disabled={busy}>{busy ? 'Checking…' : 'Ask these questions'}</button>}
      {result && (
        <section aria-label="Result">
          <p role="status"><Stars n={result.stars} size={16} /> You asked {result.found} of the {result.found + result.missed} questions that matter and {result.wasted} that did not.</p>
          <p className="row-btns"><button type="button" onClick={again}>Try again</button><button type="button" onClick={onDone}>Done</button></p>
        </section>
      )}
    </form>
  )
}
