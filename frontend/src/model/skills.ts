import type { ScenarioSummary } from '../api/types'

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

const DIFFICULTY = ['Tutorial', 'Easy', 'Medium', 'Hard']

export interface SkillRow {
  concept: string
  label: string
  total: number // challenges that practise it
  solved: number // of those, with at least one star
  stars: number
  maxStars: number
  next?: string // the challenge to try next: lowest stars, easiest first; none once all have three
}

/** One row for each concept some challenge is tagged with. A pure function of the challenge list and the stars earned. */
export function skillMap(list: ScenarioSummary[], progress: Record<string, number>): SkillRow[] {
  const by = new Map<string, ScenarioSummary[]>()
  for (const s of list) for (const c of s.concepts ?? []) by.set(c, [...(by.get(c) ?? []), s])
  const known = Object.keys(CONCEPT_LABELS)
  const order = [...known.filter((c) => by.has(c)), ...[...by.keys()].filter((c) => !known.includes(c)).sort()]
  return order.map((concept) => {
    const items = by.get(concept)!
    const stars = (s: ScenarioSummary) => progress[s.id] ?? 0
    const candidates = items.filter((s) => stars(s) < 3).sort((a, b) => stars(a) - stars(b) || DIFFICULTY.indexOf(a.difficulty) - DIFFICULTY.indexOf(b.difficulty))
    return {
      concept,
      label: CONCEPT_LABELS[concept] ?? concept,
      total: items.length,
      solved: items.filter((s) => stars(s) > 0).length,
      stars: items.reduce((n, s) => n + stars(s), 0),
      maxStars: items.length * 3,
      next: candidates[0]?.id,
    }
  })
}
