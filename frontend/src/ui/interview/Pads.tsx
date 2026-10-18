import { calculate, estimateNotes, humanBytes, humanCount, humanRate, payloadWarnings } from '../../model/estimate'
import type { EstimateInputs, InterviewData, Requirement } from '../../model/interview'
import { starterDiagram } from '../../model/starter'
import { MAX_ROWS } from '../../model/interview'
import { addRow, apiHints, blankEndpoint, blankTable, dataHints, removeRow, setRow } from '../../model/pads'
import type { StageId } from '../../model/timer'
import { useState } from 'react'
import { runDiagram } from '../../api/client'
import { checkTargets, rubricEvidence, type TargetRow } from '../../model/rubric'
import { RUBRIC_AREAS } from '../../model/interview'
import { buildMarkdown, fileStamp } from '../../model/exportDoc'
import { diagramPng, download, slug } from './exportFiles'
import { useStore } from '../../state/store'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']
const UNITS = ['%', 'ms', 'req/s', 'GB', 'TB', 'users']

/** The tool for the current stage: a pad of plain lists. Everything typed here is kept in this browser only. */
export function StagePad({ stage, data }: { stage: StageId; data: InterviewData }) {
  const set = useStore((s) => s.setInterview)
  const update = (patch: Partial<InterviewData>) => set({ ...data, ...patch })
  if (stage === 'requirements') return <Requirements data={data} update={update} />
  if (stage === 'estimation') return <Calculator data={data} update={update} />
  if (stage === 'api') return <ApiPad data={data} update={update} />
  if (stage === 'data') return <DataPad data={data} update={update} />
  if (stage === 'design' || stage === 'deepdive' || stage === 'tradeoffs') {
    return (
      <div>
        <label className="pad-notes">Notes <span className="muted small">(kept in this browser only)</span>
          <textarea value={data.notes} maxLength={5000} rows={3} onChange={(e) => update({ notes: e.target.value })} />
        </label>
        {(stage === 'design' || stage === 'deepdive') && <TargetsCheck data={data} />}
        {stage === 'tradeoffs' && <RubricPanel data={data} update={update} />}
        {stage === 'tradeoffs' && <ExportPanel data={data} />}
      </div>
    )
  }
  return null
}

type Props = { data: InterviewData; update: (p: Partial<InterviewData>) => void }

function Requirements({ data, update }: Props) {
  const nf = data.nonFunctional
  const setNf = (i: number, patch: Partial<Requirement>) => update({ nonFunctional: setRow(nf, i, { ...nf[i], ...patch }) })
  return (
    <div className="pad two-col">
      <fieldset>
        <legend>Functional: what the system does</legend>
        {data.functional.map((f, i) => (
          <div className="pad-row" key={i}>
            <input aria-label={`Functional requirement ${i + 1}`} value={f} maxLength={200} onChange={(e) => update({ functional: setRow(data.functional, i, e.target.value) })} />
            <button aria-label={`Remove functional requirement ${i + 1}`} className="x" onClick={() => update({ functional: removeRow(data.functional, i) })}>×</button>
          </div>
        ))}
        <button onClick={() => update({ functional: addRow(data.functional, '') })} disabled={data.functional.length >= MAX_ROWS}>Add a functional requirement</button>
        {data.functional.length < 3 && <p className="muted small">Aim for three.</p>}
      </fieldset>
      <fieldset>
        <legend>Non-functional: how well, with numbers</legend>
        {nf.map((r, i) => (
          <div className="pad-row" key={i}>
            <input aria-label={`Non-functional requirement ${i + 1}`} value={r.label} maxLength={200} placeholder="e.g. availability" onChange={(e) => setNf(i, { label: e.target.value })} />
            <input aria-label={`Target number for requirement ${i + 1}`} className="num-in" inputMode="decimal" value={r.target ?? ''} onChange={(e) => setNf(i, { target: e.target.value === '' ? null : Number(e.target.value) || 0 })} />
            <select aria-label={`Unit for requirement ${i + 1}`} value={r.unit} onChange={(e) => setNf(i, { unit: e.target.value })}>
              <option value="">unit</option>
              {UNITS.map((u) => <option key={u} value={u}>{u}</option>)}
            </select>
            <button aria-label={`Remove non-functional requirement ${i + 1}`} className="x" onClick={() => update({ nonFunctional: removeRow(nf, i) })}>×</button>
          </div>
        ))}
        <button onClick={() => update({ nonFunctional: addRow(nf, { label: '', target: null, unit: '' }) })} disabled={nf.length >= MAX_ROWS}>Add a non-functional requirement</button>
        {nf.filter((r) => r.target === null).length > 0 && <p className="muted small">A requirement without a number cannot be tested.</p>}
      </fieldset>
    </div>
  )
}

function ApiPad({ data, update }: Props) {
  const rows = data.api
  const hints = apiHints(rows)
  return (
    <div className="pad">
      {rows.map((e, i) => (
        <div className="pad-row wrap" key={i}>
          <select aria-label={`Method of endpoint ${i + 1}`} value={e.method} onChange={(ev) => update({ api: setRow(rows, i, { ...e, method: ev.target.value }) })}>{METHODS.map((m) => <option key={m}>{m}</option>)}</select>
          <input aria-label={`Path of endpoint ${i + 1}`} value={e.path} maxLength={200} placeholder="/photos/{id}" onChange={(ev) => update({ api: setRow(rows, i, { ...e, path: ev.target.value }) })} />
          <input aria-label={`Request of endpoint ${i + 1}`} value={e.request} placeholder="in" onChange={(ev) => update({ api: setRow(rows, i, { ...e, request: ev.target.value }) })} />
          <input aria-label={`Response of endpoint ${i + 1}`} value={e.response} placeholder="out" onChange={(ev) => update({ api: setRow(rows, i, { ...e, response: ev.target.value }) })} />
          <input aria-label={`Notes of endpoint ${i + 1}`} value={e.notes} placeholder="notes" onChange={(ev) => update({ api: setRow(rows, i, { ...e, notes: ev.target.value }) })} />
          <button aria-label={`Remove endpoint ${i + 1}`} className="x" onClick={() => update({ api: removeRow(rows, i) })}>×</button>
        </div>
      ))}
      <button onClick={() => update({ api: addRow(rows, blankEndpoint()) })} disabled={rows.length >= MAX_ROWS}>Add an endpoint</button>
      {hints.length > 0 && <ul className="hints small" aria-label="Questions to answer">{hints.map((h) => <li key={h}>{h}</li>)}</ul>}
    </div>
  )
}

function DataPad({ data, update }: Props) {
  const rows = data.tables
  const hints = dataHints(rows)
  return (
    <div className="pad">
      {rows.map((t, i) => (
        <div className="pad-row wrap" key={i}>
          <input aria-label={`Name of table ${i + 1}`} value={t.name} maxLength={100} placeholder="table" onChange={(ev) => update({ tables: setRow(rows, i, { ...t, name: ev.target.value }) })} />
          <input aria-label={`Fields of table ${i + 1}`} value={t.fields} placeholder="fields" onChange={(ev) => update({ tables: setRow(rows, i, { ...t, fields: ev.target.value }) })} />
          <input aria-label={`Key of table ${i + 1}`} value={t.key} maxLength={200} placeholder="key" onChange={(ev) => update({ tables: setRow(rows, i, { ...t, key: ev.target.value }) })} />
          <input aria-label={`Indexes of table ${i + 1}`} value={t.indexes} placeholder="indexes" onChange={(ev) => update({ tables: setRow(rows, i, { ...t, indexes: ev.target.value }) })} />
          <input aria-label={`Access patterns of table ${i + 1}`} value={t.access} placeholder="queries" onChange={(ev) => update({ tables: setRow(rows, i, { ...t, access: ev.target.value }) })} />
          <button aria-label={`Remove table ${i + 1}`} className="x" onClick={() => update({ tables: removeRow(rows, i) })}>×</button>
        </div>
      ))}
      <button onClick={() => update({ tables: addRow(rows, blankTable()) })} disabled={rows.length >= MAX_ROWS}>Add a table</button>
      {hints.length > 0 && <ul className="hints small" aria-label="Questions to answer">{hints.map((h) => <li key={h}>{h}</li>)}</ul>}
    </div>
  )
}

const FIELDS: { key: keyof EstimateInputs; label: string; unit: string }[] = [
  { key: 'mau', label: 'Monthly users', unit: 'users' },
  { key: 'dauPct', label: 'Active each day', unit: '%' },
  { key: 'actionsPerDay', label: 'Actions per user per day', unit: '' },
  { key: 'peakFactor', label: 'Peak is how many times the average', unit: '×' },
  { key: 'readPct', label: 'Reads', unit: '%' },
  { key: 'bytesEach', label: 'Size of one stored record', unit: 'bytes' },
  { key: 'retentionYears', label: 'Kept for', unit: 'years' },
  { key: 'replication', label: 'Copies of the data', unit: '' },
  { key: 'payloadBytes', label: 'Size of one response', unit: 'bytes' },
  { key: 'perServerQps', label: 'One server handles', unit: 'req/s' },
  { key: 'headroomPct', label: 'Spare capacity to keep', unit: '%' },
]

function Calculator({ data, update }: Props) {
  const importDiagram = useStore((s) => s.importDiagram)
  const requestFit = useStore((s) => s.requestFit)
  const stage = useStore((s) => s.interviewStage)
  const toast = useStore((s) => s.toast)
  const est = data.estimate
  const r = calculate(est)
  const set = (k: keyof EstimateInputs, v: string) => {
    const n = Number(v.replace(/,/g, ''))
    update({ estimate: { ...est, [k]: Number.isFinite(n) ? n : est[k] } })
  }
  const out: [string, string][] = [
    ['Daily users', humanCount(r.dau)],
    ['Average load', humanRate(r.avgQps)],
    ['Peak load', humanRate(r.peakQps)],
    ['Reads / writes (average)', `${humanRate(r.readQps)} / ${humanRate(r.writeQps)}`],
    ['Storage kept', humanBytes(r.storageBytes)],
    ['Peak bandwidth', Number.isFinite(r.peakGbps) ? `${r.peakGbps.toFixed(2)} Gbit/s` : '—'],
    ['App servers needed', Number.isFinite(r.servers) ? String(r.servers) : '—'],
  ]
  return (
    <div className="pad two-col">
      <fieldset>
        <legend>Your numbers (change any)</legend>
        {FIELDS.map((f) => (
          <label className="calc-row" key={f.key}>{f.label} {f.unit && <span className="muted small">({f.unit})</span>}
            <input inputMode="decimal" value={est[f.key]} onChange={(e) => set(f.key, e.target.value)} aria-label={`${f.label}${f.unit ? ', in ' + f.unit : ''}`} />
          </label>
        ))}
      </fieldset>
      <fieldset aria-live="polite">
        <legend>Results</legend>
        <dl className="kv">{out.map(([k, v]) => (<><dt key={k}>{k}</dt><dd key={k + v}>{v}</dd></>))}</dl>
        <div className="row-btns">
          <button onClick={() => { update({ notes: data.notes ? `${data.notes}\n\n${estimateNotes(r)}` : estimateNotes(r) }); toast('Copied into your notes', 'info', 2500) }}>Copy into notes</button>
          <button className="primary" disabled={!Number.isFinite(r.servers)} onClick={() => { importDiagram(starterDiagram({ peakQps: r.peakQps, readPct: est.readPct, servers: r.servers })); requestFit(); stage('design') }}>Build a starter from my numbers</button>
        </div>
        {payloadWarnings(est.payloadBytes).map((w) => <p key={w} className="small warn-note" role="note"><b>Payload size.</b> {w}</p>)}
        <p className="muted small">A starter is users, a balancer, {Number.isFinite(r.servers) ? Math.min(12, r.servers) : 'N'} app server(s) and a database. Your previous canvas stays one undo away.</p>
      </fieldset>
    </div>
  )
}

function ExportPanel({ data }: { data: InterviewData }) {
  const diagram = useStore((s) => s.diagram)
  const reg = useStore((s) => s.reg)
  const toast = useStore((s) => s.toast)
  const [title, setTitle] = useState('My design')
  const [theme, setTheme] = useState<'light' | 'dark'>('light')
  if (!reg) return null
  const stamp = fileStamp(new Date())
  const base = `${slug(title)}-${stamp}`
  const png = async () => {
    const blob = await diagramPng(diagram, reg, theme)
    if (!blob) return toast('The canvas is empty: nothing to export', 'warn', 3000)
    download(blob, `${base}.png`)
  }
  const md = () => download(new Blob([buildMarkdown(data, diagram, reg, { title, date: stamp, image: diagram.nodes.length ? `${base}.png` : undefined })], { type: 'text/markdown' }), `${base}.md`)
  return (
    <fieldset className="pad export">
      <legend>Take it with you</legend>
      <label>Title <input value={title} maxLength={80} onChange={(e) => setTitle(e.target.value)} aria-label="Title of the write-up" /></label>{' '}
      <label>Picture <select value={theme} onChange={(e) => setTheme(e.target.value as 'light' | 'dark')} aria-label="Background of the picture"><option value="light">Light background</option><option value="dark">Dark background</option></select></label>{' '}
      <button onClick={() => void png()}>Download the diagram (PNG)</button>
      <button onClick={md}>Download the write-up (Markdown)</button>
    </fieldset>
  )
}

/** Runs the design for 30 simulated seconds and compares what it measured with the numbers the learner wrote down. */
function TargetsCheck({ data }: { data: InterviewData }) {
  const diagram = useStore((s) => s.diagram)
  const [rows, setRows] = useState<TargetRow[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const testable = data.nonFunctional.length > 0
  const go = async () => {
    setBusy(true)
    setError('')
    try {
      const r = await runDiagram(diagram)
      const run = r.stats.global.run
      setRows(checkTargets(data.nonFunctional, { success: run.resolved ? run.ok / run.resolved : 0, p95: run.p95 }))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'The test failed.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <fieldset className="pad">
      <legend>Does my design meet my own requirements?</legend>
      <button onClick={() => void go()} disabled={busy || !testable || diagram.nodes.length === 0}>{busy ? 'Running…' : 'Test it against my requirements'}</button>
      {!testable && <span className="muted small"> Write non-functional requirements first.</span>}
      {diagram.nodes.length === 0 && testable && <span className="muted small"> The canvas is empty.</span>}
      {error && <p className="bad small" role="alert">{error}</p>}
      {rows && (
        <table className="sweep-table">
          <caption className="sr">Your requirements against a 30-second run of your design</caption>
          <thead><tr><th scope="col">Requirement</th><th scope="col">Target</th><th scope="col">Measured</th><th scope="col">Result</th></tr></thead>
          <tbody>{rows.map((r, i) => <tr key={i}><th scope="row">{r.label || '(unnamed)'}</th><td>{r.target}</td><td>{r.actual}</td><td>{r.ok === null ? 'Not tested' : r.ok ? 'Met' : 'Missed'}</td></tr>)}</tbody>
        </table>
      )}
    </fieldset>
  )
}

const AREA_LABEL: Record<string, string> = { scalability: 'Scalability', availability: 'Availability', consistency: 'Consistency', cost: 'Cost', communication: 'Communication' }

/** A 1-5 self-rating per area, each next to facts from the design, the lint and the last run. */
function RubricPanel({ data, update }: Props) {
  const diagram = useStore((s) => s.diagram)
  const findings = useStore((s) => s.findings)
  const stats = useStore((s) => s.stats.global)
  const run = stats.run.resolved > 0 ? { success: stats.run.ok / stats.run.resolved, p95: stats.run.p95, costPerHour: stats.avgCost || stats.cost } : null
  const facts = rubricEvidence(diagram, { data, findings, run })
  return (
    <fieldset className="pad rubric">
      <legend>Rate your design (1 to 5), with the evidence beside it</legend>
      {RUBRIC_AREAS.map((a) => (
        <div className="rubric-row" key={a}>
          <div role="group" aria-label={`${AREA_LABEL[a]} rating`} className="seg">
            <span className="rubric-name">{AREA_LABEL[a]}</span>
            {[1, 2, 3, 4, 5].map((n) => (
              <button key={n} aria-pressed={data.rubric[a] === n} className={data.rubric[a] === n ? 'on' : ''} aria-label={`${AREA_LABEL[a]}: ${n} out of 5`} onClick={() => update({ rubric: { ...data.rubric, [a]: data.rubric[a] === n ? 0 : n } })}>{n}</button>
            ))}
          </div>
          <ul className="small evidence">
            {facts.filter((f) => f.area === a).map((f) => <li key={f.text}><b>{f.ok === null ? 'Measured:' : f.ok ? 'Good:' : 'Gap:'}</b> {f.text}</li>)}
          </ul>
        </div>
      ))}
    </fieldset>
  )
}
