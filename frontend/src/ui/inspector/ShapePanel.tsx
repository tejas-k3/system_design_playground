import { MAX_SHAPE, MAX_SHAPE_LABEL, MIN_SHAPE, shapeInfo } from '../../model/shapes'
import { useStore } from '../../state/store'

export function ShapePanel({ id }: { id: string }) {
  const shape = useStore((s) => s.diagram.shapes?.find((g) => g.id === id))
  const updateShape = useStore((s) => s.updateShape)
  const del = useStore((s) => s.deleteIds)
  if (!shape) return null
  const arrow = shape.kind === 'arrow'
  const num = (set: (n: number) => void) => (e: React.ChangeEvent<HTMLInputElement>) => Number.isFinite(e.target.valueAsNumber) && set(e.target.valueAsNumber)
  return (
    <div className="panel">
      <h2>{shapeInfo(shape.kind).name}</h2>
      <div className="field">
        <label htmlFor="sl">Caption (optional)</label>
        <input id="sl" type="text" value={shape.label} maxLength={MAX_SHAPE_LABEL} onChange={(e) => updateShape(id, { label: e.target.value })} />
      </div>
      <div className="field">
        <label htmlFor="sw">{arrow ? 'Tip: across (px, negative = left)' : 'Width'}</label>
        <input id="sw" type="number" min={arrow ? -MAX_SHAPE : MIN_SHAPE} max={MAX_SHAPE} step={20} value={Math.round(shape.w)} onChange={num((w) => updateShape(id, { w }))} />
      </div>
      <div className="field">
        <label htmlFor="sh">{arrow ? 'Tip: down (px, negative = up)' : 'Height'}</label>
        <input id="sh" type="number" min={arrow ? -MAX_SHAPE : MIN_SHAPE} max={MAX_SHAPE} step={20} value={Math.round(shape.h)} onChange={num((h) => updateShape(id, { h }))} />
      </div>
      <p className="muted small">{arrow ? 'A free arrow for pointing at something; drag either end, or use the arrow keys to move it. It is not a connection.' : 'An ellipse to circle part of the design; drag its outline to move it and its corner to resize.'} The simulation ignores it.</p>
      <button className="danger-outline" onClick={() => del([id])}>Delete {shapeInfo(shape.kind).name.toLowerCase()}</button>
    </div>
  )
}
