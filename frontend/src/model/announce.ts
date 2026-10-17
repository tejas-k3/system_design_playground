import type { RunStats } from '../api/types'

/** One sentence a screen reader can speak about the run so far. */
export function statusSummary(run: RunStats, reasonLabel: (id: string) => string): string {
  if (run.resolved === 0) return 'No requests have finished yet.'
  const rate = ((Math.floor((run.ok / run.resolved) * 1000) / 10).toFixed(1))
  let text = `${run.resolved.toLocaleString('en-US')} requests, ${rate}% succeeded, median ${Math.round(run.p50)} ms, 95th percentile ${Math.round(run.p95)} ms, `
  if (run.failed === 0) return text + 'no failures.'
  const top = Object.entries(run.errors).sort((a, b) => b[1] - a[1])[0]
  text += `${run.failed.toLocaleString('en-US')} failed`
  return top ? `${text}, mostly ${reasonLabel(top[0])}.` : `${text}.`
}

/** Text alternative for a canvas chart: its title, unit and the latest value of each line. */
export function describeChart(title: string, unit: string, latest: { label: string; value: number }[]): string {
  const head = `${title} chart, ${unit}.`
  if (latest.length === 0) return `${head} No data yet.`
  const n = (v: number) => (v >= 10 ? `${Math.round(v)}` : `${+v.toFixed(1)}`)
  return `${head} Latest: ${latest.map((l) => `${l.label} ${n(l.value)}`).join(', ')}.`
}
