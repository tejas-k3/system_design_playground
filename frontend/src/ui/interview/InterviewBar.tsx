import { useEffect, useRef, useState } from 'react'
import { countdown, markText, stageAfterKey, stageInfo } from '../../model/stages'
import { STAGES, crossedMarks, elapsedMs, remainingMs, stageTimes } from '../../model/timer'
import { useStore } from '../../state/store'
import { StagePad } from './Pads'

/** The interview frame: seven stage tabs, a countdown that is computed from timestamps, and what to do in this stage. */
export function InterviewBar() {
  const iv = useStore((s) => s.interview)
  const stage = useStore((s) => s.interviewStage)
  const setPaused = useStore((s) => s.interviewPause)
  const end = useStore((s) => s.endInterview)
  const [now, setNow] = useState(() => Date.now())
  const [spoken, setSpoken] = useState('')
  const lastElapsed = useRef<number | null>(null)
  const session = iv?.session ?? null

  useEffect(() => {
    if (!session) {
      lastElapsed.current = null
      return
    }
    const tick = () => {
      const t = Date.now()
      setNow(t)
      const el = elapsedMs(session, t)
      if (lastElapsed.current !== null) {
        const marks = crossedMarks(session, lastElapsed.current, el)
        if (marks.length) setSpoken(markText(marks[marks.length - 1]))
      }
      lastElapsed.current = el
    }
    tick()
    const id = setInterval(tick, 500)
    document.addEventListener('visibilitychange', tick) // catch up the moment the tab is shown again
    return () => {
      clearInterval(id)
      document.removeEventListener('visibilitychange', tick)
    }
  }, [session])

  if (!session) return null
  const left = remainingMs(session, now)
  const info = stageInfo(session.stage)
  const per = stageTimes(session, now)
  const mins = (ms: number) => `${Math.floor(ms / 60_000)} min`
  return (
    <section className="interview-bar" aria-label="Interview">
      <div className="iv-row">
        <div className="seg stages" role="tablist" aria-label="Interview stages">
          {STAGES.map((s, i) => (
            <button
              key={s.id}
              role="tab"
              id={`stage-${s.id}`}
              aria-selected={session.stage === s.id}
              tabIndex={session.stage === s.id ? 0 : -1}
              className={session.stage === s.id ? 'on' : ''}
              title={`${s.label}: ${mins(per[s.id])} so far`}
              onClick={() => stage(s.id)}
              onKeyDown={(e) => {
                const next = stageAfterKey(s.id, e.key)
                if (next) {
                  e.preventDefault()
                  stage(next)
                  document.getElementById(`stage-${next}`)?.focus()
                }
              }}
            >
              {i + 1}. {s.label}
            </button>
          ))}
        </div>
        <span className={`iv-clock ${left === 0 ? 'bad' : ''}`} role="timer" aria-label="Time left">{countdown(left)}</span>
        <button onClick={() => setPaused(session.pausedAt === null)} aria-pressed={session.pausedAt !== null}>{session.pausedAt === null ? 'Pause' : 'Resume'}</button>
        <button onClick={end} title="End the interview and keep your notes">End</button>
      </div>
      <p className="iv-prompt" role="tabpanel" aria-labelledby={`stage-${session.stage}`}>{info.prompt} <span className="muted small">Tool: {info.tool}.</span></p>
      {iv && <div className="iv-tool"><StagePad stage={session.stage} data={iv} /></div>}
      <div className="sr-only" role="status" aria-live="polite" aria-atomic="true">{spoken}</div>
    </section>
  )
}
