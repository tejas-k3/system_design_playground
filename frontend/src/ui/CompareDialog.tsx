import { useState } from 'react'
import { runDiagram, type RunResult } from '../api/client'
import type { SeriesPoint } from '../api/types'
import { metricRows, type RunSummary } from '../model/compare'
import { useStore } from '../state/store'
import { useEscapeKey } from './useEscapeKey'

const summary = (r: RunResult): RunSummary => ({ ...r.stats.global.run, costPerHour: r.costPerHour })

export function CompareDialog() {
  const open = useStore((s) => s.ui.compare)
  const setUi = useStore((s) => s.setUi)
  const diagram = useStore((s) => s.diagram)
  const pinned = useStore((s) => s.pinned)
  const setPinned = useStore((s) => s.setPinned)
  const scenarioId = useStore((s) => s.scenario?.def.id)
  const [runs, setRuns] = useState<{ a: RunResult; b: RunResult } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const close = () => setUi({ compare: false })
  useEscapeKey(open, close)
  if (!open) return null
  const run = async () => {
    if (!pinned) return
    setBusy(true)
    setError('')
    try {
      const [a, b] = await Promise.all([runDiagram(pinned, scenarioId), runDiagram(diagram, scenarioId)])
      setRuns({ a, b })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'The comparison failed.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Compare two designs" onClick={close}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head"><h2>Compare two designs</h2><button className="x" autoFocus onClick={close} aria-label="Close">×</button></header>
        <p className="muted small">Pin the design as it is now (A), change it, then run both with the same seed{scenarioId ? " and the challenge's traffic" : ' for 30 simulated seconds'}. B is whatever is on the canvas.</p>
        <p className="row-btns">
          <button onClick={() => { setPinned(diagram); setRuns(null) }}>{pinned ? 'Re-pin the current design as A' : 'Pin the current design as A'}</button>
          <button className="primary" onClick={run} disabled={busy || !pinned}>{busy ? 'Running…' : 'Run A and B'}</button>
        </p>
        <p className="muted small" role="status">{pinned ? `A: ${pinned.nodes.length} components pinned. B: ${diagram.nodes.length} components on the canvas.` : 'Nothing pinned yet.'}</p>
        {error && <p className="bad small" role="alert">{error}</p>}
        {runs && (
          <>
            <table className="sweep-table compare-table">
              <caption className="sr">Metrics for design A and design B</caption>
              <thead><tr><th scope="col">Metric</th><th scope="col">A (dashed)</th><th scope="col">B (solid)</th><th scope="col">B − A</th><th scope="col">Verdict</th></tr></thead>
              <tbody>
                {metricRows(summary(runs.a), summary(runs.b)).map((r) => (
                  <tr key={r.label}><th scope="row">{r.label}</th><td>{r.a}</td><td>{r.b}</td><td>{r.delta}</td><td>{r.verdict}</td></tr>
                ))}
              </tbody>
            </table>
            <p className="muted small">Latency counts successful requests only, so a design that fails more can look faster: read it together with success.</p>
            <div className="compare-charts">
              <Overlay title="Success over time (%)" a={runs.a.stats.series} b={runs.b.stats.series} value={(p) => p.successRate * 100} max={100} />
              <Overlay title="p95 latency over time (ms)" a={runs.a.stats.series} b={runs.b.stats.series} value={(p) => p.p95} />
            </div>
          </>
        )}
      </div>
    </div>
  )
}

const W = 270
const H = 130

/** One metric over time for both runs: A dashed in orange, B solid in blue, so line style carries the meaning. */
function Overlay({ title, a, b, value, max }: { title: string; a: SeriesPoint[]; b: SeriesPoint[]; value: (p: SeriesPoint) => number; max?: number }) {
  const tMax = Math.max(1, ...a.map((p) => p.t), ...b.map((p) => p.t))
  const yMax = max ?? Math.max(1, ...a.map(value), ...b.map(value))
  const path = (s: SeriesPoint[]) => s.map((p, i) => `${i ? 'L' : 'M'}${(28 + (p.t / tMax) * (W - 34)).toFixed(1)} ${(8 + (1 - value(p) / yMax) * (H - 26)).toFixed(1)}`).join(' ')
  const last = (s: SeriesPoint[]) => (s.length ? Math.round(value(s[s.length - 1])) : 0)
  return (
    <figure className="overlay-fig">
    <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${title}. At the end A is ${last(a)} and B is ${last(b)}.`} className="overlay">
      <text x="2" y="12" fontSize="10" fill="currentColor">{Math.round(yMax)}</text>
      <text x="2" y={H - 18} fontSize="10" fill="currentColor">0</text>
      <line x1="28" y1={H - 18} x2={W - 6} y2={H - 18} stroke="currentColor" opacity=".4" />
      <path d={path(a)} fill="none" stroke="#e8590c" strokeWidth="2" strokeDasharray="6 4" />
      <path d={path(b)} fill="none" stroke="#3b5bdb" strokeWidth="2" />
    </svg>
    <figcaption className="muted small">{title} · 0–{Math.round(tMax)} s · A dashed, B solid</figcaption>
    </figure>
  )
}
