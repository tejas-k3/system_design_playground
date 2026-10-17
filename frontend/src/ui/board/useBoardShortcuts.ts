import { useEffect } from 'react'
import { useStore } from '../../state/store'

const isTyping = (t: EventTarget | null) =>
  t instanceof HTMLElement && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)

interface Actions {
  /** Fit the whole design in view. */
  fit: () => void
  /** Zoom around the middle of the canvas by this factor. */
  zoomBy: (factor: number) => void
  /** Cancel any in-progress gesture (called on Escape, before the selection is cleared). */
  cancel: () => void
}

/** Global keyboard shortcuts for editing, ignored while typing in a field. */
export function useBoardShortcuts({ fit, zoomBy, cancel }: Actions): void {
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (isTyping(e.target)) return
      const st = useStore.getState()
      const mod = e.metaKey || e.ctrlKey
      const key = e.key.toLowerCase()
      if (mod && key === 'z') {
        e.preventDefault()
        if (e.shiftKey) st.redo()
        else st.undo()
      } else if (mod && key === 'y') {
        e.preventDefault()
        st.redo()
      } else if (e.key === 'Delete' || e.key === 'Backspace') {
        e.preventDefault()
        st.deleteSelection()
      } else if (e.key === 'Escape') {
        cancel()
        st.setSelection([])
      } else if (mod) {
        return
      } else if (key === 'v') st.setTool('select')
      else if (key === 'c') st.setTool('connect')
      else if (key === 'f') fit()
      else if (e.key === '+' || e.key === '=') zoomBy(1.2)
      else if (e.key === '-') zoomBy(1 / 1.2)
    }
    window.addEventListener('keydown', down)
    return () => window.removeEventListener('keydown', down)
  }, [fit, zoomBy, cancel])
}
