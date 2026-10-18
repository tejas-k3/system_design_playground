import { useEffect, useState } from 'react'
import { answerChoice, answerEstimate, buildEstimate, fetchQuestion, runCandidate, submitReview } from '../api/client'
import type { BuildResult, ChoiceResult, EstimateResult, Question, ReviewResult, RunSummary, SeriesPoint, Symptoms } from '../api/types'
import { percent, settingRows } from '../model/review'
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
  const [built, setBuilt] = useState<BuildResult | null>(null)
  const [marked, setMarked] = useState<string[]>([])
  const [review, setReview] = useState<ReviewResult | null>(null)
  const reg = useStore((s) => s.reg)
  useEffect(() => {
    setQ(null)
    setText({})
    setResult(null)
    setError('')
    setChoice(null)
    setPicked(null)
    setRuns({})
    setRunning(null)
    setMarked([])
    setReview(null)
    setBuilt(null)
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
  const build = async () => {
    if (!q || !result) return
    setBusy(true)
    setError('')
    try {
      const answers: Record<string, number> = {}
      for (const f of result.fields) answers[f.key] = f.given
      setBuilt(await buildEstimate(q.id, answers))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not build that.')
    } finally {
      setBusy(false)
    }
  }
  const submitMarks = async () => {
    if (!q) return
    setBusy(true)
    setError('')
    try {
      const r = await submitReview(q.id, marked)
      setReview(r)
      recordStars(q.id, r.stars)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not score that.')
    } finally {
      setBusy(false)
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
            {q.mode === 'review' && q.diagram && reg && (
              <>
                <form onSubmit={(e) => { e.preventDefault(); void submitMarks() }}>
                  <fieldset disabled={!!review}>
                    <legend>Tick every component you would change</legend>
                    {q.diagram.nodes.map((n) => {
                      const rows = settingRows(q.diagram!, n, reg)
                      const was = review ? (review.found.some((d) => d.nodes.includes(n.id)) ? 'found' : review.missed.some((d) => d.nodes.includes(n.id)) ? 'missed' : review.falseMarks.includes(n.id) ? 'false' : '') : ''
                      return (
                        <div key={n.id} className={`option review-node ${was}`}>
                          <label>
                            <input type="checkbox" checked={marked.includes(n.id)} onChange={(e) => setMarked(e.target.checked ? [...marked, n.id] : marked.filter((x) => x !== n.id))} /> <b>{n.label}</b> <span className="muted small">({reg.spec(n.kind)?.name ?? n.kind})</span>
                            {was === 'found' && <b> (a real flaw, found)</b>}
                            {was === 'missed' && <b> (a real flaw, missed)</b>}
                            {was === 'false' && <b> (nothing wrong here)</b>}
                          </label>
                          {rows.length > 0 && (
                            <details>
                              <summary className="small">Settings ({rows.length})</summary>
                              <dl className="kv small">{rows.map((r) => (<><dt key={r.label}>{r.label}</dt><dd key={r.label + r.value}>{r.value}{r.changed ? ' ·set' : ''}</dd></>))}</dl>
                            </details>
                          )}
                        </div>
                      )
                    })}
                  </fieldset>
                  <p className="muted small">Connections: {q.diagram.edges.map((e) => `${q.diagram!.nodes.find((n) => n.id === e.from)?.label ?? e.from} → ${q.diagram!.nodes.find((n) => n.id === e.to)?.label ?? e.to}`).join(', ')}</p>
                  {error && <p className="bad small" role="alert">{error}</p>}
                  {!review && <button className="primary" type="submit" disabled={busy}>{busy ? 'Scoring…' : 'Check my review'}</button>}
                </form>
                {review && (
                  <section aria-label="Result">
                    <p role="status"><Stars n={review.stars} size={16} /> Found {review.found.length} of {review.found.length + review.missed.length} flaws; {review.falseMarks.length} {review.falseMarks.length === 1 ? 'mark was' : 'marks were'} on components with nothing wrong. Precision {percent(review.precision)}, recall {percent(review.recall)}.</p>
                    <ul className="workings">
                      {review.found.map((d) => <li key={d.id}><b>Found.</b> {d.explain}</li>)}
                      {review.missed.map((d) => <li key={d.id}><b>Missed.</b> {d.explain}</li>)}
                    </ul>
                    <p className="row-btns"><button onClick={() => { setReview(null); setMarked([]); setError('') }}>Try again</button><button onClick={close}>Done</button></p>
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
                {q.buildHint && (
                  <div>
                    <p className="muted small">{q.buildHint}</p>
                    {!built && <button onClick={() => void build()} disabled={busy}>{busy ? 'Building…' : 'Build it and test it'}</button>}
                    {error && <p className="bad small" role="alert">{error}</p>}
                    {built && (
                      <section aria-label="Build and test">
                        <p role="status"><b>{built.verdict === 'under' ? 'Under-provisioned.' : built.verdict === 'over' ? 'Over-provisioned.' : 'About right.'}</b> {built.verdict === 'under' ? `Your numbers gave ${built.servers} server(s); the real peak needs about ${built.rightServers}, and ${Math.round((1 - built.yours.success) * 100)}% of requests failed.` : built.verdict === 'over' ? `Your numbers gave ${built.servers} servers where ${built.rightServers} are enough: it works, but costs $${built.yours.costPerHour.toFixed(2)} an hour against $${built.rightSized.costPerHour.toFixed(2)}.` : `${built.servers} server(s) carried the real peak.`}</p>
                        <table className="sweep-table">
                          <caption className="sr">Your fleet against the right-sized fleet, at the real peak load</caption>
                          <thead><tr><th scope="col">Fleet</th><th scope="col">Servers</th><th scope="col">Success</th><th scope="col">p95</th><th scope="col">Cost per hour</th></tr></thead>
                          <tbody>
                            {([['Built from your numbers', built.servers, built.yours], ['Sized from the true value', built.rightServers, built.rightSized]] as const).map(([name, n, r]) => (
                              <tr key={name}><th scope="row">{name}</th><td>{n}</td><td>{(r.success * 100).toFixed(1)}%</td><td>{Math.round(r.p95)} ms</td><td>${r.costPerHour.toFixed(2)}</td></tr>
                            ))}
                          </tbody>
                        </table>
                      </section>
                    )}
                  </div>
                )}
                <p className="row-btns"><button onClick={() => { setResult(null); setError(''); setBuilt(null) }}>Try again</button><button onClick={close}>Done</button></p>
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
