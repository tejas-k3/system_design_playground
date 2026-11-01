import type { Level, ScenarioSummary } from '../api/types'

/** The learner's level: one of the three, or "all" (the default, which hides nothing). */
export type LearnerLevel = Level | 'all'

export const LEARNER_LEVELS: { id: LearnerLevel; label: string }[] = [
  { id: 'all', label: 'All levels' },
  { id: 'sde1', label: 'SDE1' },
  { id: 'sde2', label: 'SDE2' },
  { id: 'sde3', label: 'SDE3' },
]

/** Text tag shown on a challenge; the meaning is in the words, never in a colour. */
export const LEVEL_TAG: Record<Level, string> = { sde1: 'SDE1', sde2: 'SDE2', sde3: 'SDE3' }

export function parseLevel(v: unknown): LearnerLevel {
  return v === 'sde1' || v === 'sde2' || v === 'sde3' ? v : 'all'
}

/** Where the printable study deck for a level is served (Markdown); "all" has none, since a deck belongs to one band. */
export const studyDeckUrl = (level: LearnerLevel): string | null => (level === 'all' ? null : `/api/study-deck?level=${level}`)

const DIFFICULTY = ['Tutorial', 'Easy', 'Medium', 'Hard']
const rank = (s: ScenarioSummary) => {
  const i = DIFFICULTY.indexOf(s.difficulty)
  return i < 0 ? DIFFICULTY.length : i
}

/** The chosen level's challenges first (by difficulty, stable), then the rest by difficulty. With "all" nothing moves. */
export function orderByLevel(list: ScenarioSummary[], level: LearnerLevel): { first: ScenarioSummary[]; other: ScenarioSummary[] } {
  if (level === 'all') return { first: [...list], other: [] }
  const first = list.filter((s) => s.level === level).sort((a, b) => rank(a) - rank(b)) // Array.sort is stable
  const other = list.filter((s) => s.level !== level).sort((a, b) => rank(a) - rank(b))
  return { first, other }
}

const KEY = 'sdp:level'

export function readLevel(): LearnerLevel {
  try {
    return parseLevel(window.localStorage.getItem(KEY))
  } catch {
    return 'all'
  }
}

export function writeLevel(l: LearnerLevel): void {
  try {
    window.localStorage.setItem(KEY, l)
  } catch {
    /* the choice just is not kept */
  }
}
