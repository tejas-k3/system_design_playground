import { useId, useRef, useState } from 'react'
import { splitMarkup, termById, type GlossaryTerm } from '../model/glossary'
import { loadScenario } from '../sim/loadScenario'

/** A word with its definition attached. Reachable by keyboard; the definition is read out when the word is focused. */
export function Term({ id, children }: { id: string; children?: string }) {
  const t = termById(id)
  if (!t) return <>{children ?? id}</>
  return <TermView term={t}>{children ?? t.name}</TermView>
}

function TermView({ term, children }: { term: GlossaryTerm; children: string }) {
  const tipId = useId()
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ left: number; top: number }>({ left: 0, top: 0 })
  const btn = useRef<HTMLButtonElement>(null)
  // fixed to the screen, so a panel that scrolls or clips its contents cannot cut the definition off
  const show = () => {
    const r = btn.current?.getBoundingClientRect()
    if (r) setPos({ left: Math.max(8, Math.min(r.left, window.innerWidth - 320)), top: r.bottom + 6 })
    setOpen(true)
  }
  return (
    <span className="term-wrap" onMouseEnter={show} onMouseLeave={() => setOpen(false)}>
      <button
        ref={btn}
        type="button"
        className="term"
        aria-describedby={tipId}
        aria-expanded={open}
        onFocus={show}
        onBlur={() => setOpen(false)}
        onKeyDown={(e) => { if (e.key === 'Escape' && open) { e.stopPropagation(); setOpen(false) } }}
      >{children}</button>
      <span role="tooltip" id={tipId} className={`term-tip ${open ? 'open' : ''}`} style={pos}>
        <b>{term.name}.</b> {term.short} <span className="muted">{term.long}</span>
        {term.challenge && <button type="button" className="term-try" onMouseDown={(e) => e.preventDefault()} onClick={() => loadScenario(term.challenge!)}>Try a challenge about it</button>}
      </span>
    </span>
  )
}

/** Authored text with {{term}} markup turned into Term words. */
export function Rich({ text }: { text: string }) {
  return <>{splitMarkup(text).map((p, i) => (typeof p === 'string' ? <span key={i}>{p}</span> : <Term key={i} id={p.term.id}>{p.text}</Term>))}</>
}
