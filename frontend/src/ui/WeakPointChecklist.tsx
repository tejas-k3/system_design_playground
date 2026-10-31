import { useState } from 'react'
import { toggled } from '../model/weak'

/** A local reflection checklist; what is ticked is discarded with the result view. */
export function WeakPointChecklist({ points }: { points: string[] }) {
  const [ticked, setTicked] = useState<string[]>([])
  if (points.length === 0) return null
  return (
    <fieldset className="weak-points">
      <legend>Weak points to consider</legend>
      {points.map((point) => (
        <label key={point}>
          <input type="checkbox" checked={ticked.includes(point)} onChange={() => setTicked((old) => toggled(old, point))} />
          {point}
        </label>
      ))}
    </fieldset>
  )
}
