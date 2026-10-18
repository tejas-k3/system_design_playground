import { useEffect, useRef, useState } from 'react'
import type { LatencyBucket, SeriesPoint, SimEvent } from '../api/types'
import { palettes } from '../draw/theme'
import { describeChart } from '../model/announce'
import { type EventMark, eventLabel, eventMarks } from '../model/events'
import { errorBudget } from '../model/slo'
import { bucketLabel, cdf, describeHistogram, quantileBucket } from '../model/histogram'
import { failedAfter, pct, successRate } from '../model/goals'
import { isNarrow } from '../model/responsive'
import { useStore } from '../state/store'
import { Errors } from './Inspector'
import { Traces } from './Traces'

interface Line {
  label: string
  color: string
  dash?: number[] // a dashed line, so two series never differ by colour alone
  value: (p: SeriesPoint) => number
}

function Chart({ title, unit, lines, series, events, theme, fixedMax, fmt }: { title: string; unit: string; lines: Line[]; series: SeriesPoint[]; events: SimEvent[]; theme: 'light' | 'dark'; fixedMax?: number; fmt?: (v: number) => string }) {
  const ref = useRef<HTMLCanvasElement>(null)
  const marks = useRef<EventMark[]>([])
  const [tip, setTip] = useState<{ x: number; text: string } | null>(null)
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
      ctx.setLineDash(l.dash ?? [])
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
    ctx.setLineDash([])
    // event markers: a dashed line with a lettered flag (the letter, not the colour, says what it is)
    marks.current = eventMarks(events, { t0, t1, left, width: pw })
    ctx.font = 'bold 10px system-ui, sans-serif'
    for (const m of marks.current) {
      ctx.strokeStyle = p.ink
      ctx.globalAlpha = 0.5
      ctx.setLineDash([3, 3])
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(m.x, top + 12)
      ctx.lineTo(m.x, top + ph)
      ctx.stroke()
      ctx.setLineDash([])
      ctx.globalAlpha = 1
      ctx.fillStyle = p.card
      ctx.strokeStyle = p.ink
      ctx.beginPath()
      ctx.roundRect(m.x - 7, top, 14, 12, 3)
      ctx.fill()
      ctx.stroke()
      ctx.fillStyle = p.ink
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'
      ctx.fillText(m.count > 1 ? '…' : m.glyph, m.x, top + 6.5)
    }
  }, [series, events, lines, theme, fixedMax, fmt])
  const hover = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    const x = e.clientX - r.left
    const near = marks.current.find((m) => Math.abs(m.x - x) <= 8)
    setTip(near ? { x: near.x, text: near.labels.join(' · ') } : null)
  }
  return (
    <figure className="chart">
      <figcaption>
        {title} <small>{unit}</small>
        <span className="legend">
          {lines.map((l) => (
            <i key={l.label}><b style={{ background: l.dash ? `repeating-linear-gradient(90deg, ${l.color} 0 5px, transparent 5px 8px)` : l.color }} />{l.label}</i>
          ))}
        </span>
      </figcaption>
      <canvas ref={ref} onMouseMove={hover} onMouseLeave={() => setTip(null)} role="img" aria-label={describeChart(title, unit, series.length ? lines.map((l) => ({ label: l.label, value: l.value(series[series.length - 1]) })) : [])} />
      {tip && <div className="chart-tip" style={{ left: Math.min(tip.x, 9999) }} role="presentation">{tip.text}</div>}
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

/** Histogram of successful-request latency with the cumulative share as a line and p50/p95/p99 called out. */
function Distribution({ buckets, p50, p95, p99, theme }: { buckets: LatencyBucket[]; p50: number; p95: number; p99: number; theme: 'light' | 'dark' }) {
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
    const n = buckets.reduce((a, b) => a + b.count, 0)
    if (!n) return
    const left = 40
    const right = 40
    const top = 8
    const bottom = 34
    const pw = w - left - right
    const ph = h - top - bottom
    const max = Math.max(...buckets.map((b) => b.count))
    const bw = pw / buckets.length
    const cum = cdf(buckets)
    const marked = new Map<number, string>([[quantileBucket(buckets, 0.5), 'p50'], [quantileBucket(buckets, 0.95), 'p95'], [quantileBucket(buckets, 0.99), 'p99']])
    ctx.font = '11px system-ui, sans-serif'
    buckets.forEach((b, i) => {
      const bh = (b.count / max) * ph
      ctx.fillStyle = p.accent
      ctx.globalAlpha = 0.75
      ctx.fillRect(left + i * bw + 2, top + ph - bh, bw - 4, bh)
      ctx.globalAlpha = 1
      ctx.fillStyle = p.muted
      ctx.textAlign = 'center'
      ctx.textBaseline = 'top'
      ctx.save()
      ctx.translate(left + i * bw + bw / 2, top + ph + 4)
      ctx.rotate(-0.5)
      ctx.textAlign = 'right'
      ctx.fillText(bucketLabel(buckets, i).replace(' ms', '').replace(' s', 's'), 4, 0)
      ctx.restore()
      const tag = marked.get(i)
      if (tag) {
        ctx.fillStyle = p.ink
        ctx.textAlign = 'center'
        ctx.textBaseline = 'bottom'
        ctx.fillText(tag, left + i * bw + bw / 2, top + ph - bh - 2)
      }
    })
    // cumulative share line, dashed so it never relies on colour to be told from the bars
    ctx.strokeStyle = p.ink
    ctx.setLineDash([5, 3])
    ctx.lineWidth = 1.6
    ctx.beginPath()
    cum.forEach((v, i) => {
      const x = left + i * bw + bw / 2
      const y = top + ph - v * ph
      i ? ctx.lineTo(x, y) : ctx.moveTo(x, y)
    })
    ctx.stroke()
    ctx.setLineDash([])
    ctx.fillStyle = p.muted
    ctx.textAlign = 'left'
    ctx.textBaseline = 'middle'
    for (const f of [0, 0.5, 1]) ctx.fillText(`${f * 100}%`, left + pw + 4, top + ph - f * ph)
    ctx.textAlign = 'right'
    ctx.fillText(`${max}`, left - 4, top + 6)
  }, [buckets, theme, p50, p95, p99])
  const text = describeHistogram(buckets)
  return (
    <div className="distribution">
      <canvas ref={ref} role="img" aria-label={text} />
      <p className="muted small">{text}</p>
    </div>
  )
}

export function Charts() {
  const [open, setOpen] = useState(!isNarrow())
  const [tab, setTab] = useState<'metrics' | 'latency' | 'traces'>('metrics')
  const series = useStore((s) => s.stats.series)
  const events = useStore((s) => s.stats.events)
  const run = useStore((s) => s.stats.global.run)
  const inFlight = useStore((s) => s.stats.global.inFlight)
  const theme = useStore((s) => s.theme)
  const status = useStore((s) => s.sim.status)
  const p = palettes[theme]
  const total = run.resolved
  const rate = successRate(run)
  const okLines: Line[] = [
    { label: 'ok', color: p.ok, value: (s) => s.okRps },
    { label: 'errors', color: p.bad, dash: [5, 3], value: (s) => s.errRps },
  ]
  const latLines: Line[] = [
    { label: 'p50', color: p.accent, value: (s) => s.p50 },
    { label: 'p95', color: p.warn, value: (s) => s.p95 },
  ]
  const succLines: Line[] = [{ label: 'success', color: p.accent, value: (s) => s.successRate * 100 }]

  return (
    <section className={`charts ${open ? '' : 'closed'}`} aria-label="Run metrics">
      <div className="charts-head">
        <button className="link" onClick={() => setOpen(!open)} aria-expanded={open} aria-label={open ? 'Collapse metrics' : 'Expand metrics'}>{open ? '▾' : '▸'}</button>
        <div className="seg" role="tablist" aria-label="Metrics view">
          <button role="tab" aria-selected={tab === 'metrics'} className={tab === 'metrics' ? 'on' : ''} onClick={() => { setTab('metrics'); setOpen(true) }}>Metrics</button>
          <button role="tab" aria-selected={tab === 'latency'} className={tab === 'latency' ? 'on' : ''} onClick={() => { setTab('latency'); setOpen(true) }}>Latency distribution</button>
          <button role="tab" aria-selected={tab === 'traces'} className={tab === 'traces' ? 'on' : ''} onClick={() => { setTab('traces'); setOpen(true) }}>Request traces</button>
        </div>
        <span className="tiles">
          <Tile label="Requests" value={run.resolved.toLocaleString()} />
          <Tile label="Success" value={rate === null ? '—' : pct(rate)} bad={total > 20 && (rate ?? 1) < 0.95} />
          <Tile label="p50" value={run.ok ? `${Math.round(run.p50)} ms` : '—'} />
          <Tile label="p95" value={run.ok ? `${Math.round(run.p95)} ms` : '—'} />
          <Tile label="p99" value={run.ok ? `${Math.round(run.p99)} ms` : '—'} />
          <Tile label="Failures took" value={failedAfter(run)} hint="Median time a failed request took, from the user's first attempt. p50 and p95 count successes only." />
          <Tile label="In flight" value={String(inFlight)} />
          <BudgetTile run={run} series={series} />
        </span>
      </div>
      {open && tab === 'traces' && <Traces />}
      {open && tab === 'latency' && <Distribution buckets={run.latencyBuckets} p50={run.p50} p95={run.p95} p99={run.p99} theme={theme} />}
      {open && tab === 'metrics' && (
        <div className="charts-body">
          {status === 'stopped' && series.length === 0 ? (
            <p className="muted center">Press Play to start sending traffic.</p>
          ) : (
            <>
              <ul className="sr-only" aria-label="Events during the run">{events.map((e, i) => <li key={i}>{`${e.t.toFixed(1)} seconds: ${eventLabel(e)}`}</li>)}</ul>
              <Chart title="Throughput" unit="req/s" lines={okLines} series={series} events={events} theme={theme} />
              <Chart title="Latency" unit="ms" lines={latLines} series={series} events={events} theme={theme} />
              <Chart title="Success" unit="%" lines={succLines} series={series} events={events} theme={theme} fixedMax={100} fmt={(v) => `${Math.round(v)}`} />
              <div className="chart errors-box">
                <h3>Why requests failed</h3>
                {Object.keys(run.errors).length ? <Errors errors={run.errors} total={run.failed} /> : <p className="muted small">No failures so far.</p>}
              </div>
            </>
          )}
        </div>
      )}
    </section>
  )
}

function Tile({ label, value, bad, hint }: { label: string; value: string; bad?: boolean; hint?: string }) {
  return (
    <span className={`tile ${bad ? 'bad' : ''}`} title={hint}>
      <small>{label}</small>
      <b>{value}</b>
    </span>
  )
}

const SLOS = [99, 99.9, 99.99]

/** Remaining error budget for an availability SLO the learner picks, with a fast-window burn rate. */
function BudgetTile({ run, series }: { run: { resolved: number; failed: number }; series: SeriesPoint[] }) {
  const [slo, setSlo] = useState(99.9)
  const b = errorBudget(run, series, slo)
  const left = `${Math.round(b.remainingFrac * 100)}% left`
  const word = b.status === 'exhausted' ? 'SLO missed' : b.status === 'burning' ? `burning ${Math.round(b.burnFast)}×` : left
  return (
    <span className={`tile budget ${b.status === 'ok' ? '' : 'bad'}`} title={`Availability SLO ${slo}%: ${b.allowed.toFixed(1)} failures allowed so far, ${b.used} so far. Burn ${b.burnFast.toFixed(1)}× over 10 s, ${b.burnSlow.toFixed(1)}× over 60 s (1× = exactly on pace).`}>
      <small>
        Error budget{' '}
        <select value={slo} onChange={(e) => setSlo(Number(e.target.value))} aria-label="Availability SLO">
          {SLOS.map((x) => <option key={x} value={x}>{x}%</option>)}
        </select>
      </small>
      <b>{word}</b>
      <span className="meter" role="meter" aria-label="Error budget remaining" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(b.remainingFrac * 100)}>
        <i style={{ width: `${b.remainingFrac * 100}%` }} />
      </span>
    </span>
  )
}
