import { useState } from 'react'
import { sweepLoad } from '../api/client'
import type { SweepPoint } from '../api/types'
import { describeCurve, findKnee } from '../model/sweep'
import { useStore } from '../state/store'
import { useEscapeKey } from './useEscapeKey'

const LOADS = [0.25, 0.5, 1, 1.5, 2, 3, 4]
const W = 520
const H = 240
const PAD = { l: 44, r: 52, t: 14, b: 30 }

export function CapacityDialog() {
  const open = useStore((s) => s.ui.capacity)
  const setUi = useStore((s) => s.setUi)
  const diagram = useStore((s) => s.diagram)
  const [points, setPoints] = useState<SweepPoint[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const close = () => setUi({ capacity: false })
  useEscapeKey(open, close)
  if (!open) return null
  const hasClient = diagram.nodes.some((n) => n.kind === 'client')
  const knee = findKnee(points)
  const run = async () => {
    setBusy(true)
    setError('')
    try {
      const r = await sweepLoad(diagram, LOADS)
      setPoints(r.points)
      if (r.truncated) setError('The sweep hit its time limit, so some points are missing.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'The sweep failed.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Capacity curve" onClick={close}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head"><h2>Capacity curve</h2><button className="x" autoFocus onClick={close} aria-label="Close">×</button></header>
        <p className="muted small">Runs your current design for 20 simulated seconds at several multiples of today's traffic and shows where it starts to bend: the first load where success falls under 99% or p95 reaches 3× the lightest load.</p>
        <p><button className="primary" onClick={run} disabled={busy || !hasClient}>{busy ? 'Running…' : points.length ? 'Run again' : 'Run the sweep'}</button>{!hasClient && <span className="muted small"> Add a Users component first.</span>}</p>
        {error && <p className="bad small" role="alert">{error}</p>}
        {points.length > 0 && (
          <>
            <p role="status">{describeCurve(points, knee)}</p>
            <CurveChart points={points} knee={knee} />
            <table className="sweep-table">
              <caption className="sr">Results by offered load</caption>
              <thead><tr><th scope="col">Load</th><th scope="col">Requests/s</th><th scope="col">Success</th><th scope="col">p50</th><th scope="col">p95</th><th scope="col">p99</th><th scope="col">Cost/hr</th></tr></thead>
              <tbody>
                {points.map((p, i) => (
                  <tr key={p.value} className={i === knee ? 'knee' : ''}>
                    <th scope="row">{p.value}×{i === knee ? ' (knee)' : ''}</th>
                    <td>{Math.round(p.rps)}</td><td>{(p.successRate * 100).toFixed(1)}%</td><td>{Math.round(p.p50)} ms</td><td>{Math.round(p.p95)} ms</td><td>{Math.round(p.p99)} ms</td><td>${p.costPerHour.toFixed(2)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </div>
    </div>
  )
}

/** Success (solid, circles, left axis) and p95 (dashed, squares, right axis) against offered load. */
function CurveChart({ points, knee }: { points: SweepPoint[]; knee: number | null }) {
  const maxX = Math.max(...points.map((p) => p.rps), 1)
  const maxY = Math.max(...points.map((p) => p.p95), 1)
  const x = (v: number) => PAD.l + (v / maxX) * (W - PAD.l - PAD.r)
  const ySucc = (v: number) => PAD.t + (1 - v) * (H - PAD.t - PAD.b)
  const yLat = (v: number) => PAD.t + (1 - v / maxY) * (H - PAD.t - PAD.b)
  const line = (f: (p: SweepPoint) => number) => points.map((p, i) => `${i ? 'L' : 'M'}${x(p.rps).toFixed(1)} ${f(p).toFixed(1)}`).join(' ')
  return (
    <svg className="curve" viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`Success and p95 latency against offered load. ${describeCurve(points, knee)}`}>
      <line x1={PAD.l} y1={H - PAD.b} x2={W - PAD.r} y2={H - PAD.b} stroke="currentColor" opacity=".4" />
      <text x={PAD.l - 6} y={PAD.t + 4} textAnchor="end" fontSize="11" fill="currentColor">100%</text>
      <text x={PAD.l - 6} y={H - PAD.b} textAnchor="end" fontSize="11" fill="currentColor">0%</text>
      <text x={W - PAD.r + 6} y={PAD.t + 4} fontSize="11" fill="currentColor">{Math.round(maxY)} ms</text>
      <text x={W - PAD.r + 6} y={H - PAD.b} fontSize="11" fill="currentColor">0</text>
      <text x={(PAD.l + W - PAD.r) / 2} y={H - PAD.b + 14} textAnchor="middle" fontSize="11" fill="currentColor">offered requests per second (max {Math.round(maxX)})</text>
      {knee !== null && (
        <g>
          <line x1={x(points[knee].rps)} x2={x(points[knee].rps)} y1={PAD.t} y2={H - PAD.b} stroke="currentColor" strokeDasharray="2 3" />
          <text x={x(points[knee].rps) + 4} y={PAD.t + 12} fontSize="11" fontWeight="700" fill="currentColor">knee</text>
        </g>
      )}
      <path d={line((p) => ySucc(p.successRate))} fill="none" stroke="#2f9e44" strokeWidth="2" />
      <path d={line((p) => yLat(p.p95))} fill="none" stroke="#e8590c" strokeWidth="2" strokeDasharray="6 4" />
      {points.map((p) => (
        <g key={p.value}>
          <circle cx={x(p.rps)} cy={ySucc(p.successRate)} r="3.5" fill="#2f9e44" />
          <rect x={x(p.rps) - 3.5} y={yLat(p.p95) - 3.5} width="7" height="7" fill="#e8590c" />
        </g>
      ))}
      <g fontSize="11" fill="currentColor">
        <circle cx={PAD.l + 8} cy={H - 8} r="3.5" fill="#2f9e44" /><text x={PAD.l + 16} y={H - 4}>success (left)</text>
        <rect x={PAD.l + 106} y={H - 11.5} width="7" height="7" fill="#e8590c" /><text x={PAD.l + 118} y={H - 4}>p95 (right, dashed)</text>
      </g>
    </svg>
  )
}
