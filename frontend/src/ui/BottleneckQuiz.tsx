import { useEffect, useState } from 'react'
import { checkBottleneck } from '../api/client'
import type { BottleneckResult } from '../api/types'
import { componentOptions, rowsWithFirst } from '../model/bottleneck'
import { useStore } from '../state/store'

const NONE = '__none__'

/** Predict which part of the design runs out first at twice today's traffic, then see it run at five loads. */
export function BottleneckQuiz() {
  const diagram = useStore((s) => s.diagram)
  const options = componentOptions(diagram)
  const [guess, setGuess] = useState('')
  const [result, setResult] = useState<BottleneckResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    setResult(null) // an answer belongs to the design it was asked about
    setError('')
  }, [diagram])
  const hasClient = diagram.nodes.some((n) => n.kind === 'client')
  const text = (id: string) => options.find((o) => o.id === id)?.text ?? id
  const ask = async () => {
    setBusy(true)
    setError('')
    try {
      setResult(await checkBottleneck(diagram, guess === NONE ? '' : guess))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'The check failed.')
    } finally {
      setBusy(false)
    }
  }
  if (!hasClient || options.length === 0) return null
  return (
    <section aria-label="Which part runs out first">
      <h3>Which part runs out first at twice the traffic?</h3>
      <p className="muted small">Name one, then the design runs at five loads up to 2× and shows which part first reaches 95% busy. A part that is only waiting on a busy neighbour does not count.</p>
      <p>
        <label>Your answer{' '}
          <select value={guess} onChange={(e) => { setGuess(e.target.value); setResult(null) }} aria-label="The part you think runs out first">
            <option value="" disabled>Choose a part…</option>
            {options.map((o) => <option key={o.id} value={o.id}>{o.text}</option>)}
            <option value={NONE}>None: it still copes at twice the traffic</option>
          </select>
        </label>{' '}
        <button className="primary" onClick={ask} disabled={busy || guess === ''}>{busy ? 'Running…' : 'Check my answer'}</button>
      </p>
      {error && <p className="bad small" role="alert">{error}</p>}
      {result && (
        <>
          <p role="status"><b>{result.correct ? 'Right.' : 'Not quite.'}</b> {result.explain}</p>
          <table className="sweep-table">
            <caption className="sr">Load against how busy the busiest part is</caption>
            <thead><tr><th scope="col">Load</th><th scope="col">Requests/s</th><th scope="col">Success</th><th scope="col">p95</th><th scope="col">Busiest part</th></tr></thead>
            <tbody>
              {rowsWithFirst(result.points, result.at).map((r) => (
                <tr key={r.load} className={r.first ? 'knee' : ''}>
                  <th scope="row">{r.load}×{r.first ? ' (first to run out)' : ''}</th>
                  <td>{Math.round(r.rps)}</td><td>{(r.successRate * 100).toFixed(1)}%</td><td>{Math.round(r.p95)} ms</td><td>{text(r.hottest)} ({Math.round(r.hottestUtil * 100)}% busy)</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </section>
  )
}
