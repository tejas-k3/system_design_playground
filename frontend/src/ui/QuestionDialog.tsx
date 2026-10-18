import { useEffect, useState } from 'react'
import { answerChoice, answerEstimate, fetchQuestion, runCandidate } from '../api/client'
import type { ChoiceResult, EstimateResult, Question, RunSummary, SeriesPoint, Symptoms } from '../api/types'
import { comparisonColumns } from '../model/tradeoff'
import { fmtMs } from '../model/trace'
import { describeSeries } from '../model/symptoms'
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
  const [choice, setChoice] = useState<number | null>(null)
  const [picked, setPicked] = useState<ChoiceResult | null>(null)
  const [runs, setRuns] = useState<Record<number, RunSummary>>({})
  const [running, setRunning] = useState<number | null>(null)
  useEffect(() => {
    setQ(null)
    setText({})
    setResult(null)
    setError('')
    setChoice(null)
    setPicked(null)
    setRuns({})
    setRunning(null)
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
  const run = async (i: number) => {
    if (!q) return
    setRunning(i)
    setError('')
    try {
      const r = await runCandidate(q.id, i)
      setRuns((prev) => ({ ...prev, [i]: r }))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'The run failed.')
    } finally {
      setRunning(null)
    }
  }
  const submitChoice = async () => {
    if (!q || choice === null) return setError('Pick one of the explanations first.')
    setBusy(true)
    setError('')
    try {
      const r = await answerChoice(q.id, choice)
      setPicked(r)
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
            {q.mode === 'diagnose' && q.symptoms && q.options && (
              <>
                <Dashboard symptoms={q.symptoms} />
                <form onSubmit={(e) => { e.preventDefault(); void submitChoice() }}>
                  <fieldset disabled={!!picked}>
                    <legend>What is happening?</legend>
                    {q.options.map((o, i) => (
                      <label key={i} className={`option ${picked && picked.answer === i ? 'right' : ''}`}>
                        <input type="radio" name="diagnosis" checked={choice === i} onChange={() => setChoice(i)} /> {o}
                        {picked && picked.answer === i && <b> (the right answer)</b>}
                        {picked && choice === i && picked.answer !== i && <b> (your answer)</b>}
                      </label>
                    ))}
                  </fieldset>
                  {error && <p className="bad small" role="alert">{error}</p>}
                  {!picked && <button className="primary" type="submit" disabled={busy}>{busy ? 'Checking…' : 'Check my diagnosis'}</button>}
                </form>
                {picked && (
                  <section aria-label="Result">
                    <p role="status"><Stars n={picked.stars} size={16} /> {picked.correct ? 'Correct.' : 'Not this time.'}</p>
                    <p>{picked.explain}</p>
                    <p className="row-btns"><button onClick={() => { setPicked(null); setChoice(null); setError('') }}>Try again</button><button onClick={close}>Done</button></p>
                  </section>
                )}
              </>
            )}
            {q.mode === 'tradeoff' && q.candidates && (
              <>
                <form onSubmit={(e) => { e.preventDefault(); void submitChoice() }}>
                  <fieldset disabled={!!picked}>
                    <legend>Run each design, then pick the one to ship</legend>
                    {q.candidates.map((c, i) => (
                      <div key={i} className={`option candidate ${picked && picked.answer === i ? 'right' : ''}`}>
                        <label>
                          <input type="radio" name="design" checked={choice === i} onChange={() => setChoice(i)} /> <b>{c.label}</b>
                          {picked && picked.answer === i && <b> (the right answer)</b>}
                          {picked && choice === i && picked.answer !== i && <b> (your answer)</b>}
                          <span className="muted small"> {c.detail}</span>
                        </label>
                        <button type="button" onClick={() => void run(i)} disabled={running !== null} aria-label={`Run: ${c.label}`}>{running === i ? 'Running…' : runs[i] ? 'Run again' : 'Run it'}</button>
                      </div>
                    ))}
                  </fieldset>
                  {Object.keys(runs).length > 0 && (
                    <table className="sweep-table">
                      <caption className="sr">Results of the designs you have run</caption>
                      <thead><tr><th scope="col">Design</th>{comparisonColumns(Object.values(runs)).map((c) => <th key={c.label} scope="col">{c.label}</th>)}</tr></thead>
                      <tbody>
                        {q.candidates.map((c, i) => runs[i] && (
                          <tr key={i}><th scope="row">{c.label}</th>{comparisonColumns(Object.values(runs)).map((col) => <td key={col.label}>{col.format(runs[i])}</td>)}</tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                  {Object.keys(runs).length > 0 && Object.keys(runs).length < q.candidates.length && <p className="muted small">{q.candidates.length - Object.keys(runs).length} design(s) not run yet.</p>}
                  {error && <p className="bad small" role="alert">{error}</p>}
                  {!picked && <button className="primary" type="submit" disabled={busy}>{busy ? 'Checking…' : 'Ship this one'}</button>}
                </form>
                {picked && (
                  <section aria-label="Result">
                    <p role="status"><Stars n={picked.stars} size={16} /> {picked.correct ? 'Good call.' : 'Not this time.'}</p>
                    <p>{picked.explain}</p>
                    <p className="row-btns"><button onClick={() => { setPicked(null); setChoice(null); setError('') }}>Try again</button><button onClick={close}>Done</button></p>
                  </section>
                )}
              </>
            )}
            {q.mode === 'estimate' && !result && (
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
            {q.mode === 'estimate' && result && (
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

const W = 250
const H = 110

function Line({ title, unit, series, lines }: { title: string; unit: string; series: SeriesPoint[]; lines: { pick: (p: SeriesPoint) => number; dash?: string; color: string }[] }) {
  const tMax = Math.max(1, ...series.map((p) => p.t))
  const yMax = Math.max(1, ...lines.flatMap((l) => series.map(l.pick)))
  const path = (pick: (p: SeriesPoint) => number) => series.map((p, i) => `${i ? 'L' : 'M'}${(30 + (p.t / tMax) * (W - 36)).toFixed(1)} ${(8 + (1 - pick(p) / yMax) * (H - 24)).toFixed(1)}`).join(' ')
  const label = lines.length > 1 ? `${describeSeries(title, series, lines[0].pick, unit)} ${describeSeries('Errors', series, lines[1].pick, unit)}` : describeSeries(title, series, lines[0].pick, unit)
  return (
    <figure className="overlay-fig">
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={label} className="overlay">
        <text x="2" y="12" fontSize="10" fill="currentColor">{Math.round(yMax)}</text>
        <text x="2" y={H - 16} fontSize="10" fill="currentColor">0</text>
        <line x1="30" y1={H - 16} x2={W - 6} y2={H - 16} stroke="currentColor" opacity=".4" />
        {lines.map((l, i) => <path key={i} d={path(l.pick)} fill="none" stroke={l.color} strokeWidth="2" strokeDasharray={l.dash} />)}
      </svg>
      <figcaption className="muted small">{title} ({unit}){lines.length > 1 ? ', errors dashed' : ''} · 0–{Math.round(tMax)} s</figcaption>
    </figure>
  )
}

/** What the on-call engineer would see: three charts, the failure reasons and one failed request. */
function Dashboard({ symptoms }: { symptoms: Symptoms }) {
  const reg = useStore((s) => s.reg)
  const reason = (id: string) => reg?.reasonLabel(id) ?? id
  const t = symptoms.trace
  return (
    <section aria-label="What the dashboard shows">
      <div className="compare-charts">
        <Line title="Requests per second" unit="req/s" series={symptoms.series} lines={[{ pick: (p) => p.okRps, color: '#2f9e44' }, { pick: (p) => p.errRps, color: '#e03131', dash: '5 3' }]} />
        <Line title="p95 latency" unit="ms" series={symptoms.series} lines={[{ pick: (p) => p.p95, color: '#3b5bdb' }]} />
        <Line title="Requests in flight" unit="requests" series={symptoms.series} lines={[{ pick: (p) => p.inFlight, color: '#e8590c' }]} />
      </div>
      <p className="small"><b>Failures by reason:</b> {Object.entries(symptoms.errors).sort((a, b) => b[1] - a[1]).map(([k, v]) => `${reason(k)} ${v.toLocaleString()}`).join(', ') || 'none'}</p>
      {t && (
        <details>
          <summary>One failed request: {t.write ? 'write' : 'read'}, {fmtMs(t.latencyMs)}, {t.attempts} {t.attempts === 1 ? 'attempt' : 'attempts'}, {t.reason ? reason(t.reason) : 'failed'}</summary>
          <ol className="workings">
            {t.spans.filter((sp) => !sp.late).map((sp, i) => (
              <li key={i} style={{ marginLeft: sp.depth * 14 }}>{sp.label}: {fmtMs(sp.end - sp.at)}{sp.start - sp.at > 1 ? `, waited ${fmtMs(sp.start - sp.at)} for a free thread` : ''} · {sp.ok ? 'ok' : sp.reason ? reason(sp.reason) : 'failed'}</li>
            ))}
          </ol>
        </details>
      )}
    </section>
  )
}
