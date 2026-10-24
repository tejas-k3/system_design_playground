import type { RunStats } from '../api/types'

export interface MetricTile {
  label: string
  value: string
  hint: string
}

/** The tiles that only an LLM design has: the wait for the first token and the gap between tokens (both p95). */
export function llmTiles(run: Pick<RunStats, 'ttftMs' | 'itlMs'>): MetricTile[] {
  const ttft = run.ttftMs ?? 0
  if (ttft <= 0) return []
  const itl = run.itlMs ?? 0
  return [
    { label: 'First token', value: `${Math.round(ttft)} ms`, hint: 'Time to first token, p95: how long a user waits, from asking, before the first word of the answer appears (queueing, then the model reading the prompt).' },
    { label: 'Token gap', value: itl > 0 ? `${Math.round(itl)} ms` : '—', hint: 'Gap between tokens while the answer streams, p95: the speed the text appears at, set by how many answers share the GPU.' },
  ]
}
