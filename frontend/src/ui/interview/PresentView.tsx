import { useEffect } from 'react'
import { countdown, stageAfterKey } from '../../model/stages'
import { presentSlide } from '../../model/present'
import { STAGES, remainingMs } from '../../model/timer'
import { useStore } from '../../state/store'

/** Present mode: the side panels give way to the current stage, large, and the arrow keys walk the stages. */
export function PresentView() {
  const on = useStore((s) => s.ui.present)
  const iv = useStore((s) => s.interview)
  const setUi = useStore((s) => s.setUi)
  const stage = useStore((s) => s.interviewStage)
  const requestFit = useStore((s) => s.requestFit)
  const session = iv?.session ?? null

  useEffect(() => {
    if (!on) return
    requestFit()
    const key = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT')) return
      const cur = useStore.getState().interview?.session?.stage
      if (e.key === 'Escape') {
        e.preventDefault()
        setUi({ present: false })
      } else if (cur && (e.key === 'ArrowRight' || e.key === 'ArrowLeft' || e.key === 'Home' || e.key === 'End')) {
        e.preventDefault()
        const next = stageAfterKey(cur, e.key)
        if (next) stage(next)
      }
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [on, setUi, stage, requestFit])

  useEffect(() => {
    if (!on) requestFit()
  }, [on, requestFit])

  if (!on || !session || !iv) return null
  const idx = STAGES.findIndex((s) => s.id === session.stage)
  const slide = presentSlide(session.stage, iv)
  const left = remainingMs(session, Date.now())
  return (
    <section className={`present ${slide.kind === 'canvas' ? 'over-canvas' : ''}`} aria-label="Presentation">
      <header className="present-head">
        <h1 aria-live="polite">{idx + 1} / {STAGES.length} · {STAGES[idx].label}</h1>
        <span className="muted" aria-hidden>{countdown(left)} left · ← → change stage · Esc leaves</span>
        <button onClick={() => setUi({ present: false })}>Exit present mode</button>
      </header>
      {slide.kind === 'lists' && (
        <div className="present-body">
          {slide.groups.map((g) => (
            <div key={g.title}>
              <h2>{g.title}</h2>
              {g.items.length ? <ul>{g.items.map((i, k) => <li key={k}>{i}</li>)}</ul> : <p className="muted">Nothing written.</p>}
            </div>
          ))}
        </div>
      )}
      {slide.kind === 'facts' && (
        <dl className="present-body facts">{slide.rows.map(([k, v]) => (<div key={k}><dt>{k}</dt><dd>{v}</dd></div>))}</dl>
      )}
      {slide.kind === 'table' && (
        <div className="present-body">
          {slide.rows.length === 0 ? <p className="muted">Nothing written.</p> : (
            <table>
              <thead><tr>{slide.head.map((h) => <th key={h} scope="col">{h}</th>)}</tr></thead>
              <tbody>{slide.rows.map((r, i) => <tr key={i}>{r.map((c, j) => (j === 0 ? <th key={j} scope="row">{c}</th> : <td key={j}>{c}</td>))}</tr>)}</tbody>
            </table>
          )}
        </div>
      )}
    </section>
  )
}
