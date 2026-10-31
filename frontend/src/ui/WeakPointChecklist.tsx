import { useState } from 'react'

/** A local reflection checklist; checked state is discarded with the result view. */
export function WeakPointChecklist({ points }: { points: string[] }) {
  const [checked, setChecked] = useState<boolean[]>(() => points.map(() => false))
  if (points.length === 0) return null
  return (
    <fieldset className="weak-points">
      <legend>Weak points to consider</legend>
      {points.map((point, i) => (
        <label key={point}>
          <input type="checkbox" checked={checked[i]} onChange={(e) => setChecked((old) => old.map((v, n) => n === i ? e.target.checked : v))} />
          {point}
        </label>
      ))}
    </fieldset>
  )
}
