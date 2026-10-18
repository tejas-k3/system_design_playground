import { useEffect, useMemo, useRef, useState } from 'react'
import type { NodeStats, ParamSpec } from '../api/types'
import { palettes } from '../draw/theme'
import { connectableTargets, paramVisible } from '../model/diagram'
import { isCompact } from '../model/responsive'
import { fmtCost, pct, successRate } from '../model/goals'
import { phaseChip } from '../model/phase'
import { sparkPoints, sparkSummary } from '../model/spark'
import { crashNode, requestNodeSeries, restartNode } from '../sim/bridge'
import { ReviewPanel } from './ReviewPanel'
import { useStore } from '../state/store'

const whole = (v: number) => `${Math.round(v * 100)}%`
const rate = (v: number) => (v >= 100 ? Math.round(v).toString() : v.toFixed(1))

export function Inspector() {
  const selection = useStore((s) => s.selection)
  const diagram = useStore((s) => s.diagram)
  const open = useStore((s) => s.ui.inspector)
  const setUi = useStore((s) => s.setUi)
  const reg = useStore((s) => s.reg)
  // below the wide layout the inspector is an overlay (drawer or bottom sheet): bring it up when a component gets selected.
  // A freshly drawn connection is selected too, but covering the canvas then would hide the very thing just built.
  useEffect(() => {
    if (isCompact() && selection.some((id) => diagram.nodes.some((n) => n.id === id))) setUi({ inspector: true })
  }, [selection, diagram.nodes, setUi])
  if (!reg) return null
  if (!open) {
    return (
      <button className="inspector-tab" onClick={() => setUi({ inspector: true })} aria-label="Open inspector">
        ‹
      </button>
    )
  }
  const only = selection.length === 1 ? selection[0] : null
  const node = only ? diagram.nodes.find((n) => n.id === only) : undefined
  const edge = only ? diagram.edges.find((e) => e.id === only) : undefined
  const note = only ? diagram.notes.find((n) => n.id === only) : undefined
  return (
    <aside className="inspector" aria-label="Inspector">
      <button className="collapse" onClick={() => setUi({ inspector: false })} aria-label="Collapse inspector" title="Collapse">›</button>
      {node ? <NodePanel id={node.id} /> : edge ? <EdgePanel id={edge.id} /> : note ? <NotePanel id={note.id} /> : <Overview count={selection.length} />}
    </aside>
  )
}

function Overview({ count }: { count: number }) {
  const diagram = useStore((s) => s.diagram)
  const cost = useStore((s) => s.cost)
  const run = useStore((s) => s.stats.global.run)
  const status = useStore((s) => s.sim.status)
  const total = run.resolved
  return (
    <div className="panel">
      <h2>{count > 1 ? `${count} items selected` : 'Overview'}</h2>
      <dl className="kv">
        <dt>Components</dt><dd>{diagram.nodes.length}</dd>
        <dt>Connections</dt><dd>{diagram.edges.length}</dd>
        <dt>Cost</dt><dd>{fmtCost(cost)}</dd>
      </dl>
      {status !== 'stopped' && total > 0 && (
        <>
          <h3>This run</h3>
          <dl className="kv">
            <dt>Requests</dt><dd>{run.resolved.toLocaleString()}</dd>
            <dt>Succeeded</dt><dd>{pct(successRate(run) ?? 0)}</dd>
            <dt>p50 / p95</dt><dd>{Math.round(run.p50)} / {Math.round(run.p95)} ms</dd>
            {(run.lostUpdates ?? 0) > 0 && (<><dt>Updates overwritten</dt><dd className="bad">{run.lostUpdates!.toLocaleString()}</dd></>)}
            {(run.txAborts ?? 0) > 0 && (<><dt>Transactions aborted</dt><dd>{run.txAborts!.toLocaleString()}{(run.deadlocks ?? 0) > 0 ? ` (${run.deadlocks!.toLocaleString()} deadlocks)` : ''}</dd></>)}
            {run.lostWrites > 0 && (<><dt>Writes lost in a crash</dt><dd className="bad">{run.lostWrites.toLocaleString()}</dd></>)}
          </dl>
          <Errors errors={run.errors} total={run.failed} />
        </>
      )}
      <ReviewPanel />
      <p className="muted small">Select a component to edit its settings. Shift-click or drag a box to select several. Press Delete to remove.</p>
    </div>
  )
}

export function Errors({ errors, total }: { errors: Record<string, number>; total: number }) {
  const reg = useStore((s) => s.reg)
  const reasonLabel = (k: string) => reg?.reasonLabel(k) ?? k
  const rows = Object.entries(errors).sort((a, b) => b[1] - a[1])
  if (!rows.length) return null
  return (
    <ul className="errors">
      {rows.map(([k, v]) => (
        <li key={k}>
          <span>{reasonLabel(k)}</span>
          <span className="bar"><i style={{ width: `${Math.max(3, (v / Math.max(1, total)) * 100)}%` }} /></span>
          <b>{v.toLocaleString()}</b>
        </li>
      ))}
    </ul>
  )
}

function NodePanel({ id }: { id: string }) {
  const node = useStore((s) => s.diagram.nodes.find((n) => n.id === id))!
  const diagram = useStore((s) => s.diagram)
  const reg = useStore((s) => s.reg)!
  const stats = useStore((s) => s.stats.nodes[id])
  const status = useStore((s) => s.sim.status)
  const updateNode = useStore((s) => s.updateNode)
  const setParam = useStore((s) => s.setParam)
  const del = useStore((s) => s.deleteIds)
  const inChallenge = useStore((s) => !!s.scenario)
  const spec = reg.spec(node.kind)!
  const resolved = reg.resolve(node)
  const [adv, setAdv] = useState(false)

  const groups = useMemo(() => {
    const m = new Map<string, ParamSpec[]>()
    for (const p of spec.params ?? []) {
      if (!paramVisible(diagram, node, p, resolved)) continue
      if (p.advanced && !adv) continue
      m.set(p.group, [...(m.get(p.group) ?? []), p])
    }
    return [...m.entries()]
  }, [spec, diagram, node, resolved, adv])
  const hasAdvanced = (spec.params ?? []).some((p) => p.advanced)

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="swatch" style={{ background: spec.accent }} />
        <input className="title-input" value={node.label} onChange={(e) => updateNode(id, { label: e.target.value })} aria-label="Component name" maxLength={40} />
      </div>
      <p className="muted small">{spec.description}</p>
      <p className="lesson">💡 {spec.lesson}</p>

      {stats && status !== 'stopped' && <LiveStats s={stats} kind={node.kind} />}
      {status !== 'stopped' && <Timelines nodeId={id} status={status} />}
      {status !== 'stopped' && node.kind !== 'client' && (
        <div className="row-btns">
          {stats?.down ? (
            <button onClick={() => restartNode(id)}>Restart</button>
          ) : (
            <button className="danger" onClick={() => crashNode(id)}>Crash it</button>
          )}
        </div>
      )}

      {groups.map(([group, ps]) => (
        <section key={group}>
          <h3>{group}</h3>
          {ps.map((p) => (
            <ParamField key={p.key} p={p} value={resolved[p.key]} locked={inChallenge && !!p.fixedInChallenge} onChange={(v) => setParam(id, p.key, v)} />
          ))}
        </section>
      ))}
      {hasAdvanced && (
        <button className="link" onClick={() => setAdv(!adv)}>{adv ? 'Hide' : 'Show'} advanced</button>
      )}
      <Connections nodeId={id} />
      {!node.locked && <button className="danger-outline" onClick={() => del([id])}>Delete component</button>}
    </div>
  )
}

/** Wire components without dragging: lists what this node sends to, and adds or removes links. */
function Connections({ nodeId }: { nodeId: string }) {
  const diagram = useStore((s) => s.diagram)
  const reg = useStore((s) => s.reg)!
  const addEdge = useStore((s) => s.addEdge)
  const deleteIds = useStore((s) => s.deleteIds)
  const setSelection = useStore((s) => s.setSelection)
  const [target, setTarget] = useState('')
  const out = diagram.edges.filter((e) => e.from === nodeId)
  const label = (id: string) => diagram.nodes.find((n) => n.id === id)?.label ?? id
  const options = connectableTargets(diagram, reg, nodeId)
  const chosen = options.some((o) => o.id === target) ? target : (options[0]?.id ?? '')
  if (out.length === 0 && options.length === 0) return null
  return (
    <section aria-label="Connections">
      <h3>Sends requests to</h3>
      {out.length === 0 && <p className="muted small">Nothing yet.</p>}
      <ul className="conns">
        {out.map((e) => (
          <li key={e.id}>
            <span>{label(e.to)}</span>
            <button className="x" aria-label={`Remove connection to ${label(e.to)}`} onClick={() => deleteIds([e.id])}>×</button>
          </li>
        ))}
      </ul>
      {options.length > 0 && (
        <div className="conn-add">
          <label className="sr" htmlFor="conn-target">Connect to</label>
          <select id="conn-target" value={chosen} onChange={(e) => setTarget(e.target.value)}>
            {options.map((o) => (
              <option key={o.id} value={o.id}>{o.label}</option>
            ))}
          </select>
          <button
            onClick={() => {
              // addEdge selects the new link; stay on this node so several links can be added in a row
              if (chosen && addEdge(nodeId, chosen)) setSelection([nodeId])
            }}
          >
            Connect
          </button>
        </div>
      )}
    </section>
  )
}

/** A tiny canvas line of one metric over the last two minutes; the text below it is what a screen reader gets. */
function Spark({ label, values, max, fmt, theme }: { label: string; values: number[]; max?: number; fmt: (v: number) => string; theme: 'light' | 'dark' }) {
  const ref = useRef<HTMLCanvasElement>(null)
  useEffect(() => {
    const cv = ref.current
    if (!cv) return
    const dpr = window.devicePixelRatio || 1
    const w = cv.clientWidth
    const h = cv.clientHeight
    cv.width = Math.round(w * dpr)
    cv.height = Math.round(h * dpr)
    const ctx = cv.getContext('2d')!
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, w, h)
    const pts = sparkPoints(values, { width: w - 2, height: h - 4, max })
    if (!pts.length) return
    const p = palettes[theme]
    ctx.strokeStyle = p.accent
    ctx.lineWidth = 1.6
    ctx.lineJoin = 'round'
    ctx.beginPath()
    pts.forEach(([x, y], i) => (i ? ctx.lineTo(x + 1, y + 2) : ctx.moveTo(x + 1, y + 2)))
    ctx.stroke()
  }, [values, max, theme])
  const text = sparkSummary(label, values, fmt)
  return (
    <div className="spark">
      <span className="small muted">{label}</span>
      <canvas ref={ref} role="img" aria-label={text} />
    </div>
  )
}

/** Per-second history of the selected component, fetched from the server every two seconds while a run exists. */
function Timelines({ nodeId, status }: { nodeId: string; status: string }) {
  const series = useStore((st) => st.nodeSeries)
  const theme = useStore((st) => st.theme)
  useEffect(() => {
    if (status === 'stopped') return
    requestNodeSeries(nodeId)
    if (status !== 'running') return
    const t = setInterval(() => requestNodeSeries(nodeId), 2000)
    return () => clearInterval(t)
  }, [nodeId, status])
  const pts = series?.nodeId === nodeId ? series.points : []
  if (pts.length < 2) return null
  return (
    <div className="timelines">
      <h3>Last {pts.length} s</h3>
      <Spark label="Busy" values={pts.map((p) => p.util)} max={1} fmt={whole} theme={theme} />
      <Spark label="Waiting" values={pts.map((p) => p.qLen)} fmt={(v) => `${Math.round(v)}`} theme={theme} />
      <Spark label="Failed per second" values={pts.map((p) => p.err)} fmt={(v) => `${rate(v)}/s`} theme={theme} />
      {pts.some((p) => p.instances) && <Spark label="Servers" values={pts.map((p) => p.instances ?? 0)} fmt={(v) => `${v}`} theme={theme} />}
    </div>
  )
}

function LiveStats({ s, kind }: { s: NodeStats; kind: string }) {
  const reg = useStore((st) => st.reg)
  const reasonLabel = (k: string) => reg?.reasonLabel(k) ?? k
  const reasons = Object.entries(s.reasons ?? {}).sort((a, b) => b[1] - a[1])
  return (
    <div className="live">
      <h3>Live</h3>
      <dl className="kv">
        <dt>In</dt><dd>{rate(s.arr)}/s</dd>
        <dt>Succeeded</dt><dd>{rate(s.ok)}/s</dd>
        <dt>Failed</dt><dd className={s.err > 0.5 ? 'bad' : ''}>{rate(s.err)}/s</dd>
        {phaseChip(s) && (<><dt>In progress</dt><dd>{phaseChip(s)}</dd></>)}
        {kind !== 'client' && kind !== 'loadBalancer' && (
          <>
            <dt>Busy</dt><dd>{whole(s.util)}{s.cap ? ` · ${s.busy}/${s.cap}` : ''}</dd>
            <dt>Waiting</dt><dd>{s.qLen}</dd>
          </>
        )}
        {s.hitRate >= 0 && (<><dt>Hit rate</dt><dd>{whole(s.hitRate)}</dd></>)}
        {s.size !== undefined && s.size > 0 && (<><dt>Cached</dt><dd>{s.size.toLocaleString()}</dd></>)}
        {s.extra?.poolWaiting !== undefined && (<><dt>Waiting for a DB connection</dt><dd>{s.extra.poolWaiting}</dd></>)}
        {s.extra?.openConns !== undefined && (<><dt>Open connections</dt><dd className={s.extra.openConns >= (s.extra.maxConns ?? 1e9) ? 'bad' : ''}>{s.extra.openConns} / {s.extra.maxConns}</dd></>)}
        {s.extra?.conflictRps !== undefined && (<><dt>Double bookings</dt><dd className="bad">{rate(s.extra.conflictRps)}/s</dd></>)}
        {s.extra?.dupRps !== undefined && (<><dt>Duplicate writes</dt><dd className="bad">{rate(s.extra.dupRps)}/s</dd></>)}
        {s.extra?.evictRps !== undefined && (<><dt>Evictions</dt><dd>{rate(s.extra.evictRps)}/s</dd></>)}
      </dl>
      {reasons.length > 0 && (
        <ul className="errors">
          {reasons.map(([k, v]) => (
            <li key={k}><span>{reasonLabel(k)}</span><span className="bar"><i style={{ width: `${Math.min(100, (v / Math.max(0.1, s.arr)) * 100)}%` }} /></span><b>{rate(v)}/s</b></li>
          ))}
        </ul>
      )}
    </div>
  )
}

function ParamField({ p, value, onChange, locked }: { p: ParamSpec; value: unknown; onChange: (v: number | string | boolean) => void; locked: boolean }) {
  const id = `p-${p.key}`
  return (
    <div className="field" title={p.help}>
      <label htmlFor={id}>
        {p.label}
        {locked && <em> · fixed</em>}
      </label>
      {p.type === 'number' && <NumberField id={id} p={p} value={Number(value)} onChange={onChange} disabled={locked} />}
      {p.type === 'select' && (
        <select id={id} value={String(value)} disabled={locked} onChange={(e) => onChange(e.target.value)}>
          {p.options?.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      )}
      {p.type === 'boolean' && (
        <label className="switch">
          <input id={id} type="checkbox" checked={Boolean(value)} disabled={locked} onChange={(e) => onChange(e.target.checked)} />
          <span>{value ? 'On' : 'Off'}</span>
        </label>
      )}
      {p.help && <small className="help">{p.help}</small>}
    </div>
  )
}

function NumberField({ id, p, value, onChange, disabled }: { id: string; p: ParamSpec; value: number; onChange: (v: number) => void; disabled: boolean }) {
  const min = p.min ?? 0
  const max = p.max ?? 1000
  const [draft, setDraft] = useState<string | null>(null)
  const commit = (raw: string) => {
    setDraft(null)
    const n = Number(raw)
    if (raw.trim() !== '' && Number.isFinite(n)) onChange(Math.min(max, Math.max(min, n)))
  }
  // log-ish slider for wide ranges
  const wide = max / Math.max(1, min) >= 200
  const toSlider = (v: number) => (wide ? Math.log(Math.max(v, min || 1) / (min || 1)) / Math.log(max / (min || 1)) : (v - min) / (max - min))
  const fromSlider = (t: number) => {
    const raw = wide ? (min || 1) * Math.pow(max / (min || 1), t) : min + t * (max - min)
    const step = p.step || 1
    return Math.min(max, Math.max(min, Math.round(raw / step) * step))
  }
  return (
    <div className="num">
      <input type="range" min={0} max={1} step={0.001} value={Math.min(1, Math.max(0, toSlider(value)))} disabled={disabled} onChange={(e) => onChange(fromSlider(Number(e.target.value)))} aria-label={p.label} />
      <input
        id={id}
        type="text"
        inputMode="decimal"
        value={draft ?? String(Number(value.toFixed(3)))}
        disabled={disabled}
        onFocus={(e) => e.currentTarget.select()}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={(e) => commit(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && (e.currentTarget as HTMLInputElement).blur()}
      />
      {p.unit && <span className="unit">{p.unit}</span>}
    </div>
  )
}

function EdgePanel({ id }: { id: string }) {
  const edge = useStore((s) => s.diagram.edges.find((e) => e.id === id))!
  const diagram = useStore((s) => s.diagram)
  const st = useStore((s) => s.stats.edges[id])
  const reg = useStore((s) => s.reg)!
  const updateEdge = useStore((s) => s.updateEdge)
  const del = useStore((s) => s.deleteIds)
  const from = diagram.nodes.find((n) => n.id === edge.from)
  const to = diagram.nodes.find((n) => n.id === edge.to)
  const [draft, setDraft] = useState<string | null>(null)
  return (
    <div className="panel">
      <h2>Connection</h2>
      <p className="small"><b>{from?.label}</b> → <b>{to?.label}</b></p>
      <p className="muted small">{reg.spec(from?.kind ?? '')?.name} to {reg.spec(to?.kind ?? '')?.name}</p>
      <div className="field">
        <label htmlFor="loss">Packet loss <span className="muted small">(%)</span></label>
        <div className="num">
          <input id="loss" type="text" inputMode="decimal" defaultValue={edge.lossPct ?? 0} key={`loss-${id}-${edge.lossPct ?? 0}`} onFocus={(e) => e.currentTarget.select()}
            onBlur={(e) => { const n = Number(e.target.value); if (Number.isFinite(n) && n >= 0) updateEdge(id, { lossPct: Math.min(100, n) || undefined }) }} />
          <span className="unit">%</span>
        </div>
        <p className="muted small">Each lost packet is sent again after 200 ms; six lost in a row abort the connection ("Connection lost").</p>
        <label htmlFor="jit">Jitter <span className="muted small">(ms)</span></label>
        <div className="num">
          <input id="jit" type="text" inputMode="decimal" defaultValue={edge.jitterMs ?? 0} key={`jit-${id}-${edge.jitterMs ?? 0}`} onFocus={(e) => e.currentTarget.select()}
            onBlur={(e) => { const n = Number(e.target.value); if (Number.isFinite(n) && n >= 0) updateEdge(id, { jitterMs: Math.min(1000, n) || undefined }) }} />
          <span className="unit">ms</span>
        </div>
        <p className="muted small">A random extra 0 to this many ms on each one-way trip.</p>
      </div>
      <div className="field">
        <label htmlFor="bw">Bandwidth <span className="muted small">(Mbit/s, 0 = unlimited)</span></label>
        <div className="num">
          <input id="bw" type="text" inputMode="decimal" defaultValue={edge.bandwidthMbps ?? 0} key={`${id}-${edge.bandwidthMbps ?? 0}`} onFocus={(e) => e.currentTarget.select()}
            onBlur={(e) => { const n = Number(e.target.value); if (Number.isFinite(n) && n >= 0) updateEdge(id, { bandwidthMbps: Math.min(100000, n) || undefined }) }} />
          <span className="unit">Mbit/s</span>
        </div>
        <p className="muted small">A limited link shares its capacity: requests wait for the bytes ahead of them. It only matters once the users have a request or response size.</p>
        {st?.mbps !== undefined && <p className="small">Carrying {Math.round(st.mbps)} Mbit/s{st.util !== undefined ? ` (${Math.round(st.util * 100)}% of the limit${st.util >= 0.95 ? ': full' : ''})` : ''}.</p>}
      </div>
      <div className="field">
        <label htmlFor="lat">One-way latency</label>
        <div className="num">
          <input type="range" min={reg.raw.latency.min} max={reg.raw.latency.max} value={edge.latencyMs} onChange={(e) => updateEdge(id, { latencyMs: Number(e.target.value) })} aria-label="Latency" />
          <input
            id="lat"
            type="text"
            inputMode="numeric"
            value={draft ?? String(edge.latencyMs)}
            onFocus={(e) => e.currentTarget.select()}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={(e) => {
              setDraft(null)
              const n = Number(e.target.value)
              if (Number.isFinite(n)) updateEdge(id, { latencyMs: Math.min(reg.raw.latency.max, Math.max(reg.raw.latency.min, Math.round(n))) })
            }}
          />
          <span className="unit">ms</span>
        </div>
        <small className="help">Each request pays this on the way there and again on the way back.</small>
      </div>
      {st && (
        <dl className="kv">
          <dt>Traffic</dt><dd>{rate(st.rps)}/s</dd>
          {st.breaker && (<><dt>Circuit breaker</dt><dd className={st.breaker === 'open' ? 'bad' : ''}>{st.breaker}</dd></>)}
          {st.ejected && (<><dt>Status</dt><dd className="bad">ejected by load balancer</dd></>)}
        </dl>
      )}
      <button className="danger-outline" onClick={() => del([id])}>Delete connection</button>
    </div>
  )
}

function NotePanel({ id }: { id: string }) {
  const note = useStore((s) => s.diagram.notes.find((n) => n.id === id))!
  const updateNote = useStore((s) => s.updateNote)
  const del = useStore((s) => s.deleteIds)
  return (
    <div className="panel">
      <h2>Note</h2>
      <textarea className="note-input" value={note.text} onChange={(e) => updateNote(id, { text: e.target.value })} rows={5} aria-label="Note text" />
      <div className="field">
        <label htmlFor="fs">Text size</label>
        <input id="fs" type="range" min={12} max={40} value={note.fontSize ?? 18} onChange={(e) => updateNote(id, { fontSize: Number(e.target.value) })} />
      </div>
      <button className="danger-outline" onClick={() => del([id])}>Delete note</button>
    </div>
  )
}
