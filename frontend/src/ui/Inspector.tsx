import { useEffect, useMemo, useState } from 'react'
import type { NodeStats, ParamSpec } from '../api/types'
import { paramVisible } from '../model/diagram'
import { isNarrow } from '../model/responsive'
import { crashNode, restartNode } from '../sim/bridge'
import { useStore } from '../state/store'

const pct = (v: number) => `${Math.round(v * 100)}%`
const rate = (v: number) => (v >= 100 ? Math.round(v).toString() : v.toFixed(1))

export function Inspector() {
  const selection = useStore((s) => s.selection)
  const diagram = useStore((s) => s.diagram)
  const open = useStore((s) => s.ui.inspector)
  const setUi = useStore((s) => s.setUi)
  const reg = useStore((s) => s.reg)
  // on a phone the inspector is a bottom sheet: bring it up when something gets selected
  useEffect(() => {
    if (isNarrow() && selection.length > 0) setUi({ inspector: true })
  }, [selection, setUi])
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
        <dt>Cost</dt><dd>${cost.toFixed(2)}/hr</dd>
      </dl>
      {status !== 'stopped' && total > 0 && (
        <>
          <h3>This run</h3>
          <dl className="kv">
            <dt>Requests</dt><dd>{run.resolved.toLocaleString()}</dd>
            <dt>Succeeded</dt><dd>{pct(run.ok / Math.max(1, total))}</dd>
            <dt>p50 / p95</dt><dd>{Math.round(run.p50)} / {Math.round(run.p95)} ms</dd>
            {run.lostWrites > 0 && (<><dt>Writes lost in a crash</dt><dd className="bad">{run.lostWrites.toLocaleString()}</dd></>)}
          </dl>
          <Errors errors={run.errors} total={run.failed} />
        </>
      )}
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
      {!node.locked && <button className="danger-outline" onClick={() => del([id])}>Delete component</button>}
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
        {kind !== 'client' && kind !== 'loadBalancer' && (
          <>
            <dt>Busy</dt><dd>{pct(s.util)}{s.cap ? ` · ${s.busy}/${s.cap}` : ''}</dd>
            <dt>Waiting</dt><dd>{s.qLen}</dd>
          </>
        )}
        {s.hitRate >= 0 && (<><dt>Hit rate</dt><dd>{pct(s.hitRate)}</dd></>)}
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
