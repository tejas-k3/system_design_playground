import { Fragment, useEffect, useState } from 'react'
import type { Trace } from '../api/types'
import { diagnose, fmtMs, layout } from '../model/trace'
import { requestTraces } from '../sim/bridge'
import { useStore } from '../state/store'

type Kind = 'failed' | 'ok'

/** Recent requests, failed ones first, and a waterfall showing where the selected one spent its time. */
export function Traces() {
  const traces = useStore((s) => s.traces)
  const status = useStore((s) => s.sim.status)
  const reg = useStore((s) => s.reg)
  const [kind, setKind] = useState<Kind>('failed')
  const [selected, setSelected] = useState<number | null>(null)
  const reasonLabel = (id: string) => reg?.reasonLabel(id) ?? id

  // fetch once when opened, then keep refreshing while the simulation runs
  useEffect(() => {
    requestTraces()
    if (status !== 'running') return
    const t = setInterval(requestTraces, 1500)
    return () => clearInterval(t)
  }, [status])

  // start on the most recent failure, or the most recent success if nothing has failed yet
  useEffect(() => {
    if (traces && traces.failed.length === 0 && traces.ok.length > 0) setKind((k) => (k === 'failed' && selected === null ? 'ok' : k))
  }, [traces, selected])

  const list = traces?.[kind] ?? []
  const current = list.find((t) => t.id === selected) ?? list[0]

  if (!traces || (traces.failed.length === 0 && traces.ok.length === 0)) {
    return <p className="muted center traces-empty">{status === 'stopped' ? 'Press Play, then come back here to follow individual requests.' : 'Collecting requests…'}</p>
  }

  return (
    <div className="traces">
      <div className="trace-list">
        <div className="seg" role="tablist" aria-label="Request outcome">
          {(['failed', 'ok'] as Kind[]).map((k) => (
            <button key={k} role="tab" aria-selected={kind === k} className={kind === k ? 'on' : ''} onClick={() => { setKind(k); setSelected(null) }}>
              {k === 'failed' ? `Failed (${traces.failed.length})` : `Succeeded (${traces.ok.length})`}
            </button>
          ))}
        </div>
        <ul>
          {list.map((t) => (
            <li key={t.id}>
              <button className={`trace-card ${t.ok ? 'ok' : 'bad'} ${current?.id === t.id ? 'sel' : ''}`} onClick={() => setSelected(t.id)}>
                <b>#{t.id}</b>
                <span>{t.write ? 'write' : 'read'} · {fmtMs(t.latencyMs)}</span>
                {!t.ok && <em>{reasonLabel(t.reason ?? '')}</em>}
              </button>
            </li>
          ))}
          {list.length === 0 && <li className="muted small">None in this run yet.</li>}
        </ul>
      </div>
      {current ? <Waterfall trace={current} reasonLabel={reasonLabel} /> : <p className="muted center">Nothing to show yet.</p>}
    </div>
  )
}

function Waterfall({ trace, reasonLabel }: { trace: Trace; reasonLabel: (id: string) => string }) {
  const d = diagnose(trace, reasonLabel)
  const { total, rows } = layout(trace)
  return (
    <div className="wf" aria-label={`Request ${trace.id}`}>
      <div className={`wf-head ${trace.ok ? 'ok' : 'bad'}`}>
        <b>{d.headline}</b>
        {d.notes.map((n) => (
          <p key={n}>{n}</p>
        ))}
      </div>
      <div className="wf-axis" aria-hidden>
        <span>0</span>
        <span>{fmtMs(total / 2)}</span>
        <span>{fmtMs(total)}</span>
      </div>
      <div className="wf-rows">
        {rows.map((r, i) => (
          <Fragment key={i}>
            {r.attempt !== undefined && <div className="wf-attempt">Attempt {r.attempt}</div>}
            <div className={`wf-row ${r.span.late ? 'late' : ''} ${r.span.node === d.culprit ? (trace.ok ? 'slowest' : 'culprit') : ''}`}>
              <span className="wf-name" style={{ paddingLeft: r.span.depth * 12 }} title={r.span.label}>{r.span.label}</span>
              <span className="wf-track">
                {r.wait > 0 && <i className="wf-wait" style={{ left: `${r.left}%`, width: `${r.wait}%` }} title="waiting in a queue" />}
                <i className={`wf-bar ${r.span.ok ? 'ok' : 'bad'} ${r.span.open ? 'open' : ''}`} style={{ left: `${r.left + r.wait}%`, width: `${r.width}%` }} />
              </span>
              <span className="wf-time">
                {fmtMs(r.span.end - r.span.at)}
                {r.span.reason && <em> · {reasonLabel(r.span.reason)}</em>}
                {r.span.late && <em> · after the user gave up</em>}
                {r.span.open && !r.span.late && <em> · still running</em>}
              </span>
            </div>
          </Fragment>
        ))}
      </div>
    </div>
  )
}
