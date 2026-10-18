import { useEffect, useState } from 'react'
import { answerEstimate, fetchQuestion } from '../api/client'
import type { EstimateResult, Question } from '../api/types'
import { formatEstimate, parseNumber, verdictLabel } from '../model/questions'
import { useStore } from '../state/store'
import { Stars } from './Stars'
import { useEscapeKey } from './useEscapeKey'

export function QuestionDialog() {
  const id = useStore((s) => s.ui.question)
  const setUi = useStore((s) => s.setUi)
  const recordStars = useStore((s) => s.recordStars)
  const [q, setQ] = useState<Question | null>(null)
  const [text, setText] = useState<Record<string, string>>({})
  const [result, setResult] = useState<EstimateResult | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    setQ(null)
    setText({})
    setResult(null)
    setError('')
    if (id) fetchQuestion(id).then(setQ).catch((e: Error) => setError(e.message))
  }, [id])
  const close = () => setUi({ question: null })
  useEscapeKey(!!id, close)
  if (!id) return null
  const submit = async () => {
    if (!q?.fields) return
    const answers: Record<string, number> = {}
    for (const f of q.fields) {
      const n = parseNumber(text[f.key] ?? '')
      if (n === null) return setError(`Give a positive number for "${f.label}" (for example 2,500 or 1.5k).`)
      answers[f.key] = n
    }
    setBusy(true)
    setError('')
    try {
      const r = await answerEstimate(q.id, answers)
      setResult(r)
      recordStars(q.id, r.stars)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not grade that.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label={q?.title ?? 'Question'} onClick={close}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head"><h2>{q?.title ?? 'Loading…'}</h2><button className="x" autoFocus onClick={close} aria-label="Close">×</button></header>
        {q && (
          <>
            <p>{q.prompt}</p>
            <dl className="kv">{q.facts?.map((f) => (<><dt key={f.label}>{f.label}</dt><dd key={f.value}>{f.value}</dd></>))}</dl>
            {!result && (
              <form onSubmit={(e) => { e.preventDefault(); void submit() }}>
                {q.fields?.map((f) => (
                  <p key={f.key}>
                    <label>{f.label} <small className="muted">({f.unit})</small>{' '}
                      <input value={text[f.key] ?? ''} onChange={(e) => setText({ ...text, [f.key]: e.target.value })} inputMode="decimal" size={14} aria-label={`${f.label}, in ${f.unit}`} />
                    </label>
                  </p>
                ))}
                {error && <p className="bad small" role="alert">{error}</p>}
                <button className="primary" type="submit" disabled={busy}>{busy ? 'Checking…' : 'Check my numbers'}</button>
              </form>
            )}
            {result && (
              <section aria-label="Result">
                <p role="status"><Stars n={result.stars} size={16} /> Score {Math.round(result.score * 100)}%. {result.stars === 3 ? 'Every number is within 30%.' : 'Read the workings below.'}</p>
                <table className="sweep-table">
                  <caption className="sr">Your answers against the expected values</caption>
                  <thead><tr><th scope="col">Number</th><th scope="col">You</th><th scope="col">Expected</th><th scope="col">Verdict</th></tr></thead>
                  <tbody>
                    {result.fields.map((f) => (<tr key={f.key}><th scope="row">{f.label}</th><td>{formatEstimate(f.given)}</td><td>{formatEstimate(f.expected)} {f.unit}</td><td>{verdictLabel(f.verdict)}</td></tr>))}
                  </tbody>
                </table>
                <ul className="workings">{result.fields.map((f) => <li key={f.key}><b>{f.label}.</b> {f.explain}</li>)}</ul>
                <p className="row-btns"><button onClick={() => { setResult(null); setError('') }}>Try again</button><button onClick={close}>Done</button></p>
              </section>
            )}
          </>
        )}
        {!q && error && <p className="bad" role="alert">{error}</p>}
      </div>
    </div>
  )
}
