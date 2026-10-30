import { useState } from 'react'
import { answerMatch } from '../api/client'
import type { MatchItem, MatchResult } from '../api/types'
import { useStore } from '../state/store'
import { Stars } from './Stars'

interface Props {
  id: string
  items: MatchItem[]
  onDone: () => void
}

/** A set of needs, each with its own component options. Nothing is graded until all are answered. */
export function MatchQuestion({ id, items, onDone }: Props) {
  const recordStars = useStore((s) => s.recordStars)
  const [choices, setChoices] = useState<(number | null)[]>(() => items.map(() => null))
  const [result, setResult] = useState<MatchResult | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const left = choices.filter((c) => c === null).length
  const submit = async () => {
    if (left > 0) return setError(`Answer every line first: ${left} ${left === 1 ? 'is' : 'are'} still open.`)
    setBusy(true)
    setError('')
    try {
      const r = await answerMatch(id, choices as number[])
      setResult(r)
      recordStars(id, r.stars)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not grade that.')
    } finally {
      setBusy(false)
    }
  }
  const again = () => {
    setChoices(items.map(() => null))
    setResult(null)
    setError('')
  }
  return (
    <form onSubmit={(e) => { e.preventDefault(); void submit() }}>
      {items.map((it, i) => {
        const res = result?.items[i]
        return (
          <fieldset key={i} disabled={!!result}>
            <legend>{i + 1}. {it.need}</legend>
            {it.options.map((o, j) => (
              <label key={j} className={`option ${res && res.answer === j ? 'right' : ''}`}>
                <input type="radio" name={`item-${i}`} checked={choices[i] === j} onChange={() => setChoices(choices.map((c, k) => (k === i ? j : c)))} /> {o}
                {res && res.answer === j && <b> (the right answer)</b>}
                {res && choices[i] === j && res.answer !== j && <b> (your answer)</b>}
              </label>
            ))}
            {res && <p className="small">{res.explain}</p>}
          </fieldset>
        )
      })}
      {error && <p className="bad small" role="alert">{error}</p>}
      {!result && <button className="primary" type="submit" disabled={busy}>{busy ? 'Checking…' : 'Check my answers'}</button>}
      {result && (
        <section aria-label="Result">
          <p role="status"><Stars n={result.stars} size={16} /> {result.correct} of {items.length} right.</p>
          <p className="row-btns"><button type="button" onClick={again}>Try again</button><button type="button" onClick={onDone}>Done</button></p>
        </section>
      )}
    </form>
  )
}
