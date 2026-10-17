import { useEffect, useRef, useState } from 'react'
import type { SeriesPoint } from '../api/types'
import { palettes } from '../draw/theme'
import { useStore } from '../state/store'
import { Errors } from './Inspector'

interface Line {
  label: string
  color: string
  value: (p: SeriesPoint) => number
}

function Chart({ title, unit, lines, series, theme, fixedMax, fmt }: { title: string; unit: string; lines: Line[]; series: SeriesPoint[]; theme: 'light' | 'dark'; fixedMax?: number; fmt?: (v: number) => string }) {
  const ref = useRef<HTMLCanvasElement>(null)
  useEffect(() => {
    const cv = ref.current
    if (!cv) return
    const dpr = window.devicePixelRatio || 1
    const w = cv.clientWidth
    const h = cv.clientHeight
    if (cv.width !== Math.round(w * dpr)) {
      cv.width = Math.round(w * dpr)
      cv.height = Math.round(h * dpr)
    }
    const ctx = cv.getContext('2d')!
    const p = palettes[theme]
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, w, h)
    const left = 38
    const top = 6
    const right = 6
    const bottom = 16
    const pw = w - left - right
    const ph = h - top - bottom
    let max = fixedMax ?? 0
    if (!fixedMax) for (const l of lines) for (const s of series) max = Math.max(max, l.value(s))
    max = Math.max(1e-6, niceMax(max))
    ctx.font = '11px system-ui, sans-serif'
    ctx.fillStyle = p.muted
    ctx.strokeStyle = p.grid
    ctx.lineWidth = 1
    ctx.textAlign = 'right'
    ctx.textBaseline = 'middle'
    for (let i = 0; i <= 3; i++) {
      const y = top + ph - (ph * i) / 3
      ctx.beginPath()
      ctx.moveTo(left, y)
      ctx.lineTo(left + pw, y)
      ctx.stroke()
      const v = (max * i) / 3
      ctx.fillText(fmt ? fmt(v) : compact(v), left - 4, y)
    }
    if (series.length < 2) return
    const t0 = series[0].t
    const t1 = series[series.length - 1].t
    const span = Math.max(1, t1 - t0)
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    ctx.fillText(`${Math.round(t0)}s`, left, h - bottom + 3)
    ctx.fillText(`${Math.round(t1)}s`, left + pw, h - bottom + 3)
    for (const l of lines) {
      ctx.strokeStyle = l.color
      ctx.lineWidth = 2
      ctx.lineJoin = 'round'
      ctx.beginPath()
      series.forEach((s, i) => {
        const x = left + ((s.t - t0) / span) * pw
        const y = top + ph - Math.min(1, l.value(s) / max) * ph
        i ? ctx.lineTo(x, y) : ctx.moveTo(x, y)
      })
      ctx.stroke()
    }
  }, [series, lines, theme, fixedMax, fmt])
  return (
    <figure className="chart">
      <figcaption>
        {title} <small>{unit}</small>
        <span className="legend">
          {lines.map((l) => (
            <i key={l.label}><b style={{ background: l.color }} />{l.label}</i>
          ))}
        </span>
      </figcaption>
      <canvas ref={ref} aria-label={`${title} chart`} />
    </figure>
  )
}

function niceMax(v: number): number {
  if (v <= 0) return 1
  const e = Math.pow(10, Math.floor(Math.log10(v)))
  const m = v / e
  return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * e
}
const compact = (v: number) => (v >= 1000 ? `${+(v / 1000).toFixed(1)}k` : v >= 10 ? `${Math.round(v)}` : `${+v.toFixed(1)}`)

export function Charts() {
  const [open, setOpen] = useState(true)
  const series = useStore((s) => s.stats.series)
  const run = useStore((s) => s.stats.global.run)
  const theme = useStore((s) => s.theme)
  const status = useStore((s) => s.sim.status)
  const p = palettes[theme]
  const total = run.resolved
  const okLines = useRef<Line[]>([])
  okLines.current = [
    { label: 'ok', color: p.ok, value: (s) => s.okRps },
    { label: 'errors', color: p.bad, value: (s) => s.errRps },
  ]
  const latLines: Line[] = [
    { label: 'p50', color: p.accent, value: (s) => s.p50 },
    { label: 'p95', color: p.warn, value: (s) => s.p95 },
  ]
  const succLines: Line[] = [{ label: 'success', color: p.accent, value: (s) => s.successRate * 100 }]

  return (
    <section className={`charts ${open ? '' : 'closed'}`} aria-label="Run metrics">
      <div className="charts-head">
        <button className="link" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? '▾' : '▸'} Metrics</button>
        <span className="tiles">
          <Tile label="Requests" value={run.resolved.toLocaleString()} />
          <Tile label="Success" value={total ? `${(Math.floor((run.ok / total) * 1000) / 10).toFixed(1)}%` : '—'} bad={total > 20 && run.ok / total < 0.95} />
          <Tile label="p50" value={run.ok ? `${Math.round(run.p50)} ms` : '—'} />
          <Tile label="p95" value={run.ok ? `${Math.round(run.p95)} ms` : '—'} />
          <Tile label="In flight" value={String(useStore.getState().stats.global.inFlight)} />
        </span>
      </div>
      {open && (
        <div className="charts-body">
          {status === 'stopped' && series.length === 0 ? (
            <p className="muted center">Press Play to start sending traffic.</p>
          ) : (
            <>
              <Chart title="Throughput" unit="req/s" lines={okLines.current} series={series} theme={theme} />
              <Chart title="Latency" unit="ms" lines={latLines} series={series} theme={theme} />
              <Chart title="Success" unit="%" lines={succLines} series={series} theme={theme} fixedMax={100} fmt={(v) => `${Math.round(v)}`} />
              <div className="chart errors-box">
                <h4>Why requests failed</h4>
                {Object.keys(run.errors).length ? <Errors errors={run.errors} total={run.failed} /> : <p className="muted small">No failures so far.</p>}
              </div>
            </>
          )}
        </div>
      )}
    </section>
  )
}

function Tile({ label, value, bad }: { label: string; value: string; bad?: boolean }) {
  return (
    <span className={`tile ${bad ? 'bad' : ''}`}>
      <small>{label}</small>
      <b>{value}</b>
    </span>
  )
}
