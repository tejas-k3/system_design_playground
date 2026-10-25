// The last good challenge list, kept in this browser so the daily card and the picker have something to show when the
// server is slow or gone. Display only: a challenge is always opened through the server, never from this copy.

import type { Level, ScenarioSummary } from '../api/types'

export const SUMMARIES_KEY = 'sdp:summaries'
export const MAX_SUMMARIES = 500
export const MAX_SUMMARIES_BYTES = 512 * 1024
const MAX_TEXT = 2000
const MAX_CONCEPTS = 20
const LEVELS: Level[] = ['sde1', 'sde2', 'sde3']

const text = (v: unknown, max = MAX_TEXT): v is string => typeof v === 'string' && v.length <= max

function one(v: unknown): ScenarioSummary | null {
  if (!v || typeof v !== 'object') return null
  const o = v as Record<string, unknown>
  if (!text(o.id, 100) || !o.id || !text(o.title, 200) || !text(o.difficulty, 40) || !text(o.summary)) return null
  const s: ScenarioSummary = { id: o.id, title: o.title, difficulty: o.difficulty, summary: o.summary }
  if (o.configOnly === true) s.configOnly = true
  if (o.open === true) s.open = true
  if (Array.isArray(o.concepts)) s.concepts = o.concepts.filter((c): c is string => text(c, 60)).slice(0, MAX_CONCEPTS)
  if (typeof o.level === 'string' && (LEVELS as string[]).includes(o.level)) s.level = o.level as Level
  return s
}

/** Strictly read a stored list: valid JSON, an array of at most MAX_SUMMARIES well-formed entries with unique ids, else nothing. */
export function parseSummaries(raw: string | null | undefined): ScenarioSummary[] | null {
  if (!raw || raw.length > MAX_SUMMARIES_BYTES) return null
  let v: unknown
  try {
    v = JSON.parse(raw)
  } catch {
    return null
  }
  if (!Array.isArray(v) || v.length === 0 || v.length > MAX_SUMMARIES) return null
  const out: ScenarioSummary[] = []
  const seen = new Set<string>()
  for (const e of v) {
    const s = one(e)
    if (!s || seen.has(s.id)) return null
    seen.add(s.id)
    out.push(s)
  }
  return out
}

export function loadSummaries(): ScenarioSummary[] | null {
  try {
    return parseSummaries(window.localStorage.getItem(SUMMARIES_KEY))
  } catch {
    return null
  }
}

export function saveSummaries(list: ScenarioSummary[]): void {
  try {
    const clean = list.slice(0, MAX_SUMMARIES).map(one).filter((s): s is ScenarioSummary => s !== null)
    if (clean.length > 0) window.localStorage.setItem(SUMMARIES_KEY, JSON.stringify(clean))
  } catch {
    /* no storage or full: the list is just not kept */
  }
}
