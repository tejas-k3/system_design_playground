import type { ScenarioSummary } from '../api/types'
import type { Level } from '../api/types'
import { LEVEL_TAG, type LearnerLevel } from './level'
import { needsReview, type ChallengeRecord } from './record'

/** The concepts the backend tags challenges with, in the order the skill map lists them, with their names. */
export const CONCEPT_LABELS: Record<string, string> = {
  capacity: 'Capacity and queueing',
  caching: 'Caching',
  replication: 'Replication and failover',
  consistency: 'Consistency and transactions',
  queues: 'Queues and async work',
  'failure-handling': 'Failure handling',
  sharding: 'Sharding and hot spots',
  'overload-control': 'Overload control',
  'storage-and-payload': 'Storage, indexes and payloads',
  autoscaling: 'Autoscaling',
}

export interface LevelRow {
  level: Level
  label: string // SDE1, SDE2, SDE3
  total: number
  solved: number
  stars: number
  maxStars: number
  text: string // the same numbers in words, for the label next to the bar
}

/** Stars earned out of the stars available at each level. Challenges with no level are left out; a level with none shows 0 of 0. */
export function levelSummary(list: ScenarioSummary[], progress: Record<string, number>): LevelRow[] {
  return (['sde1', 'sde2', 'sde3'] as Level[]).map((level) => {
    const items = list.filter((s) => s.level === level)
    const stars = (s: ScenarioSummary) => Math.max(0, Math.min(3, Math.floor(progress[s.id] ?? 0)))
    const solved = items.filter((s) => stars(s) > 0).length
    const earned = items.reduce((n, s) => n + stars(s), 0)
    return { level, label: LEVEL_TAG[level], total: items.length, solved, stars: earned, maxStars: items.length * 3, text: `${solved} of ${items.length} challenges solved · ${earned} of ${items.length * 3} stars` }
  })
}

const DIFFICULTY =['Tutorial', 'Easy', 'Medium', 'Hard']

export interface SkillRow {
  concept: string
  label: string
  total: number // challenges that practise it
  solved: number // of those, with at least one star
  stars: number
  maxStars: number
  review: string[] // passed challenges here that needed the answer or had a wrong prediction
  next?: string // the challenge to try next: lowest stars, easiest first; none once all have three
}

/** One row for each concept some challenge is tagged with. A pure function of the challenge list and the stars earned. */
export function skillMap(list: ScenarioSummary[], progress: Record<string, number>, level: LearnerLevel = 'all', records: Record<string, ChallengeRecord> = {}): SkillRow[] {
  const by = new Map<string, ScenarioSummary[]>()
  for (const s of list) for (const c of s.concepts ?? []) by.set(c, [...(by.get(c) ?? []), s])
  const known = Object.keys(CONCEPT_LABELS)
  const order = [...known.filter((c) => by.has(c)), ...[...by.keys()].filter((c) => !known.includes(c)).sort()]
  return order.map((concept) => {
    const items = by.get(concept)!
    const stars = (s: ScenarioSummary) => progress[s.id] ?? 0
    const mine = (s: ScenarioSummary) => (level !== 'all' && s.level === level ? 0 : 1) // the chosen level first, then the old rule
    const candidates = items.filter((s) => stars(s) < 3).sort((a, b) => mine(a) - mine(b) || stars(a) - stars(b) || DIFFICULTY.indexOf(a.difficulty) - DIFFICULTY.indexOf(b.difficulty))
    return {
      concept,
      label: CONCEPT_LABELS[concept] ?? concept,
      total: items.length,
      solved: items.filter((s) => stars(s) > 0).length,
      stars: items.reduce((n, s) => n + stars(s), 0),
      maxStars: items.length * 3,
      next: candidates[0]?.id,
      review: items.filter((s) => needsReview(records[s.id])).map((s) => s.id),
    }
  })
}
