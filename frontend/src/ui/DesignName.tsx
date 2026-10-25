import { useRef, useState } from 'react'
import { DEFAULT_DESIGN_NAME, MAX_DESIGN_NAME, designName, normalizeDesignName } from '../model/designName'
import { useStore } from '../state/store'

/**
 * The design's title, editable in place: click it, type, Enter saves, Escape cancels, leaving the field saves.
 * An empty name goes back to "Untitled design". The pencil says it can be edited.
 */
export function DesignName() {
  const stored = useStore((s) => s.diagram.name)
  const set = useStore((s) => s.setDiagramName)
  const [draft, setDraft] = useState<string | null>(null)
  const cancelled = useRef(false)
  const shown = designName({ name: stored })
  const input = useRef<HTMLInputElement>(null)

  const commit = () => {
    if (draft === null) return
    const n = normalizeDesignName(draft)
    set(n === DEFAULT_DESIGN_NAME ? '' : n)
    setDraft(null)
  }
  return (
    <label className="design-name" title="Click to rename this design. Enter saves, Escape cancels.">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
        <path d="M4 20h4L19 9l-4-4L4 16zM14 6l4 4" />
      </svg>
      <input
        ref={input}
        type="text"
        aria-label="Design name"
        value={draft ?? shown}
        maxLength={MAX_DESIGN_NAME}
        spellCheck={false}
        onFocus={(e) => {
          cancelled.current = false
          setDraft(shown)
          e.currentTarget.select()
        }}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => {
          if (cancelled.current) setDraft(null)
          else commit()
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            input.current?.blur() // saves
          } else if (e.key === 'Escape') {
            e.preventDefault()
            e.stopPropagation() // do not also clear the canvas selection
            cancelled.current = true
            input.current?.blur()
          }
        }}
      />
    </label>
  )
}
