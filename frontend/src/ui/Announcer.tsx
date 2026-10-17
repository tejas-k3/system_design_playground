import { useEffect, useState } from 'react'
import { statusSummary } from '../model/announce'
import { useStore } from '../state/store'

const EVERY_MS = 10000

/** A visually hidden live region for screen-reader users: says when the simulation starts, pauses or stops, and,
 *  while it runs, a one-sentence summary of how it is going every ten seconds (the canvas and charts are silent). */
export function Announcer() {
  const status = useStore((s) => s.sim.status)
  const reg = useStore((s) => s.reg)
  const [text, setText] = useState('')
  useEffect(() => {
    const summary = () => {
      const st = useStore.getState()
      return statusSummary(st.stats.global.run, (id) => st.reg?.reasonLabel(id) ?? id)
    }
    if (status === 'stopped') return setText('')
    if (status === 'paused') return setText(`Paused. ${summary()}`)
    setText('Simulation running.')
    const t = setInterval(() => setText(summary()), EVERY_MS)
    return () => clearInterval(t)
  }, [status, reg])
  return <div className="sr-only" role="status" aria-live="polite" aria-atomic="true">{text}</div>
}
