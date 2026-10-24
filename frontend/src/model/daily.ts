// The daily challenge: one scenario per calendar day, the same for everyone with the same list, and a local streak of
// consecutive days on which it was solved. Pure functions over dates written YYYY-MM-DD (the player's local date); the
// only storage is the small streak record, kept apart from the main save.

import type { ScenarioSummary } from '../api/types'
import { parseLevel, type LearnerLevel } from './level'

const DAY_MS =86_400_000
const DATE = /^\d{4}-\d{2}-\d{2}$/

/** Days since 1970-01-01 for a YYYY-MM-DD date, or NaN when it is not one. */
export function dayNumber(date: string): number {
  if (!DATE.test(date)) return NaN
  const t = Date.parse(`${date}T00:00:00Z`)
  return Number.isNaN(t) ? NaN : Math.floor(t / DAY_MS)
}

function mulberry(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

// One round of the rotation: every challenge exactly once, in an order fixed by the round number.
function round(sorted: string[], cycle: number): string[] {
  const rnd = mulberry(cycle * 2654435761 + sorted.length)
  const out = [...sorted]
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(rnd() * (i + 1))
    ;[out[i], out[j]] = [out[j], out[i]]
  }
  return out
}

/** The challenge for a date: all of them take a turn before any repeats, and never the same one two days running. */
export function dailyChallenge(date: string, ids: string[]): string | undefined {
  const day = dayNumber(date)
  if (Number.isNaN(day) || ids.length === 0) return undefined
  const sorted = [...ids].sort()
  const n = sorted.length
  const cycle = Math.floor(day / n)
  const r = round(sorted, cycle)
  // when a round would start with yesterday's challenge, its first two days trade places
  if (n > 2 && r[0] === round(sorted, cycle - 1)[n - 1] && day % n <= 1) return r[1 - (day % n)]
  return r[day % n]
}

/** The daily challenge for a learner level: it rotates inside that level's challenges. "All", an unknown level, or a level with no challenges behaves exactly like dailyChallenge over every id. */
export function dailyForLevel(date: string, list: ScenarioSummary[], level: LearnerLevel): string | undefined {
  const l = parseLevel(level)
  const own = l === 'all' ? [] : list.filter((s) => s.level === l)
  return dailyChallenge(date, (own.length ? own : list).map((s) => s.id))
}

export interface Streak {
  last: string // the last date the daily challenge was solved
  count: number
}

/** The streak after solving the daily challenge on `date`. */
export function nextStreak(s: Streak | undefined, date: string): Streak {
  if (!s) return { last: date, count: 1 }
  if (s.last === date) return s
  return { last: date, count: dayNumber(date) - dayNumber(s.last) === 1 ? s.count + 1 : 1 }
}

/** The streak to display on `today`: it is still alive if the last solve was today or yesterday. */
export function shownStreak(s: Streak | undefined, today: string): number {
  if (!s) return 0
  const gap = dayNumber(today) - dayNumber(s.last)
  return gap === 0 || gap === 1 ? s.count : 0
}

const KEY = 'sdp:daily'

export function loadStreak(): Streak | undefined {
  try {
    const o = JSON.parse(window.localStorage.getItem(KEY) ?? 'null') as Partial<Streak> | null
    if (o && typeof o.last === 'string' && !Number.isNaN(dayNumber(o.last)) && typeof o.count === 'number' && o.count >= 1) return { last: o.last, count: Math.floor(o.count) }
  } catch {
    /* no storage, or a damaged value: start again */
  }
  return undefined
}

export function saveStreak(s: Streak): void {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(s))
  } catch {
    /* the streak just is not kept */
  }
}

/** Today's date in the player's own time zone, as YYYY-MM-DD. */
export function localDate(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}
