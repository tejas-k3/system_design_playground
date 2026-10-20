import { useEffect, useState } from 'react'
import { fetchAlternatives } from '../api/client'
import type { Alternative } from '../api/types'

/** After a challenge is solved: what would have happened had you only done something else, as measured. */
export function Alternatives({ id }: { id: string }) {
  const [list, setList] = useState<Alternative[]>([])
  useEffect(() => {
    let live = true
    fetchAlternatives(id).then((l) => live && setList(l)).catch(() => live && setList([]))
    return () => { live = false }
  }, [id])
  if (list.length === 0) return null
  return (
    <section aria-label="Alternatives compared" className="alts">
      <h3>What if you had only done this instead?</h3>
      <p className="muted small">Each row starts from the broken design and changes one idea. The numbers are from the simulator with its standard seed.</p>
      <table className="checks">
        <thead><tr><th scope="col">Instead</th><th scope="col">Result</th><th scope="col">Success</th><th scope="col">p95</th><th scope="col">Cost</th></tr></thead>
        <tbody>
          {list.map((a) => (
            <tr key={a.label} className={a.pass ? 'pass' : 'fail'}>
              <td><b>{a.label}</b><br /><span className="muted small">{a.why}</span></td>
              <td>{a.pass ? '✓ Passes' : '✕ Fails'}</td>
              <td>{a.successPct.toFixed(1)}%</td>
              <td>{a.successPct === 0 ? '—' : `${Math.round(a.p95Ms)} ms`}</td>
              <td>${a.costPerHour.toFixed(2)}/hr</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
