import type { Span, Trace } from '../api/types'

export const fmtMs = (v: number): string => (v >= 10000 ? `${(v / 1000).toFixed(1)} s` : `${Math.round(v)} ms`)

/**
 * Time each span spent on its own: its duration minus the time its direct dependencies took.
 * Spans arrive in call order, so a span's dependencies are the deeper spans that follow it.
 * The span's own time includes network travel to and from its dependencies.
 */
export function selfTimes(spans: Span[]): number[] {
  return spans.map((s, i) => {
    let children = 0
    for (let j = i + 1; j < spans.length && spans[j].depth > s.depth; j++) {
      if (spans[j].depth === s.depth + 1) children += spans[j].end - spans[j].at
    }
    return Math.max(0, s.end - s.at - children)
  })
}

export interface Diagnosis {
  headline: string
  notes: string[]
  /** Node id most responsible for the outcome, if any. */
  culprit?: string
}

const waited = (s: Span) => Math.max(0, s.start - s.at)

/** Say in plain words what happened to one request and where the time or the failure came from. */
export function diagnose(t: Trace, reasonLabel: (id: string) => string): Diagnosis {
  const notes: string[] = []
  const self = selfTimes(t.spans)
  let slow = -1
  t.spans.forEach((s, i) => {
    if (!s.late && !s.open && (slow < 0 || self[i] > self[slow])) slow = i
  })
  const slowNote = (verb: string) => {
    if (slow < 0) return
    const s = t.spans[slow]
    const queue = waited(s)
    let line = `${verb} ${s.label} (${fmtMs(self[slow])})`
    if (queue > 0.5 * self[slow] && queue >= 20) line += `, mostly waiting in its queue for ${fmtMs(queue)}`
    notes.push(line + '.')
  }
  const lost = t.spans.filter((s) => s.late || s.open).length
  const tries = t.attempts > 1 ? `${t.attempts} attempts were made, and every retry adds load.` : ''

  if (t.ok) {
    slowNote('Most of the time went to')
    if (t.attempts > 1) notes.push(`It needed ${t.attempts} attempts.`)
    return { headline: `Succeeded in ${fmtMs(t.latencyMs)}`, notes, culprit: slow >= 0 ? t.spans[slow].node : undefined }
  }

  if (t.reason === 'timeout') {
    slowNote('The slowest place was')
    if (lost > 0) notes.push(`${lost} step${lost === 1 ? '' : 's'} kept working after the user gave up, using capacity for nobody.`)
    if (tries) notes.push(tries)
    return { headline: `Gave up after ${fmtMs(t.latencyMs)} (timeout)`, notes, culprit: slow >= 0 ? t.spans[slow].node : undefined }
  }

  // deepest failing span is where the failure started; shallower ones just passed it up
  let blame: Span | undefined
  for (const s of t.spans) {
    if (!s.ok && !s.open && !s.late && (!blame || s.depth >= blame.depth)) blame = s
  }
  if (!blame) return { headline: `Failed: ${reasonLabel(t.reason ?? '')}`, notes: tries ? [tries] : [] }
  if (waited(blame) >= 5) notes.push(`It waited ${fmtMs(waited(blame))} before being turned away.`)
  if (tries) notes.push(tries)
  return { headline: `Failed at ${blame.label}: ${reasonLabel(blame.reason ?? t.reason ?? '')}`, notes, culprit: blame.node }
}

export interface Row {
  span: Span
  /** Percent of the timeline: where the bar starts, how much is queueing, how much is serving. */
  left: number
  wait: number
  width: number
  /** Set on the first row of each retry so the waterfall can label attempts. */
  attempt?: number
}

/** Place spans on a shared time axis (0 to total ms) for the waterfall chart. */
export function layout(t: Trace): { total: number; rows: Row[] } {
  const total = Math.max(1, t.latencyMs, ...t.spans.map((s) => s.end))
  const pct = (v: number) => (v / total) * 100
  let prev = 0
  const rows = t.spans.map((span): Row => {
    const row: Row = {
      span,
      left: pct(span.at),
      wait: pct(waited(span)),
      width: Math.max(0.8, pct(span.end - span.start)),
    }
    if (t.attempts > 1 && span.attempt !== prev) row.attempt = span.attempt
    prev = span.attempt
    return row
  })
  return { total, rows }
}
