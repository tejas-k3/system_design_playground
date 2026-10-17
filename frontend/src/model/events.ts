import type { SimEvent } from '../api/types'

const GLYPHS: Record<string, string> = {
  crash: 'X', restart: 'R', slow: 'S', slowEnd: 'N', partition: 'P', heal: 'H',
  failoverDetected: 'D', promotion: 'F', splitbrain: 'B', scaleUp: '+', serverUp: 'U', scaleDown: '-',
}

/** One-character cue drawn on a chart marker, so events never depend on colour alone. */
export const eventGlyph = (type: string): string => GLYPHS[type] ?? '?'

const writes = (n: number) => `${n} writes lost`

/** What happened, in plain words (also the accessible text for the chart markers). */
export function eventLabel(e: SimEvent): string {
  switch (e.type) {
    case 'crash': return `${e.label} crashed`
    case 'restart': return `${e.label} restarted`
    case 'slow': return `${e.label} slowed down`
    case 'slowEnd': return `${e.label} back to normal speed`
    case 'partition': return `${e.label} cut off from the network`
    case 'heal': return `${e.label} reconnected`
    case 'failoverDetected': return `Failure of ${e.label} noticed`
    case 'promotion': return `${e.label} promoted to primary (${e.lost ? writes(e.lost) : 'no writes lost'})`
    case 'splitbrain': return `${e.label} rejoined as primary: ${writes(e.lost ?? 0)}`
    case 'scaleUp': return `${e.label}: another server is booting`
    case 'serverUp': return `${e.label}: another server is running`
    case 'scaleDown': return `${e.label}: a server was removed`
    default: return `${e.label}: ${e.type}`
  }
}

export interface EventMark {
  x: number // pixel position on the chart
  glyph: string
  labels: string[]
  count: number
}

/** Chart markers for events inside the visible time window; markers closer than minGapPx are merged into one. */
export function eventMarks(events: SimEvent[], g: { t0: number; t1: number; left: number; width: number }, minGapPx = 14): EventMark[] {
  const span = g.t1 - g.t0
  if (!(span > 0)) return []
  const out: EventMark[] = []
  for (const e of [...events].sort((a, b) => a.t - b.t)) {
    if (e.t < g.t0 || e.t > g.t1) continue
    const x = g.left + ((e.t - g.t0) / span) * g.width
    const last = out[out.length - 1]
    if (last && x - last.x < minGapPx) {
      last.labels.push(eventLabel(e))
      last.count++
    } else out.push({ x, glyph: eventGlyph(e.type), labels: [eventLabel(e)], count: 1 })
  }
  return out
}
