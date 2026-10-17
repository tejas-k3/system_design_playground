import { useEffect } from 'react'

/** Call onEscape when Escape is pressed, while `active`. Lets dialogs be dismissed from the keyboard. */
export function useEscapeKey(active: boolean, onEscape: () => void): void {
  useEffect(() => {
    if (!active) return
    const down = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onEscape()
    }
    window.addEventListener('keydown', down)
    return () => window.removeEventListener('keydown', down)
  }, [active, onEscape])
}
