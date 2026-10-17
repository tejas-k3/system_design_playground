import { useStore } from '../../state/store'

/** Zoom out, the current zoom (click to fit the design on screen), zoom in. */
export function ZoomBadge({ fit, by }: { fit: () => void; by: (factor: number) => void }) {
  const zoom = useStore((s) => s.viewport.zoom)
  return (
    <div className="zoom-badge" role="group" aria-label="Zoom">
      <button onClick={() => by(1 / 1.2)} aria-label="Zoom out" title="Zoom out (-)">−</button>
      <button className="zoom-val" onClick={fit} title="Fit to content (F)">{Math.round(zoom * 100)}%</button>
      <button onClick={() => by(1.2)} aria-label="Zoom in" title="Zoom in (+)">+</button>
    </div>
  )
}
