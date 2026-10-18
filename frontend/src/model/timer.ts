// The interview session clock. Everything is computed from timestamps handed in by the caller (Date.now()), never
// from counting interval ticks, so a hidden tab that is throttled for ten minutes still reads exactly ten minutes.

export const STAGES = [
  { id: 'requirements', label: 'Clarify requirements' },
  { id: 'estimation', label: 'Estimate the scale' },
  { id: 'api', label: 'Sketch the API' },
  { id: 'data', label: 'Model the data' },
  { id: 'design', label: 'High-level design' },
  { id: 'deepdive', label: 'Deep dive' },
  { id: 'tradeoffs', label: 'Trade-offs and wrap-up' },
] as const

export type StageId = (typeof STAGES)[number]['id']

interface Interval {
  from: number
  to: number
}

export interface Session {
  totalMs: number
  startedAt: number
  pausedAt: number | null // when the current pause began, if paused
  pauses: Interval[] // pauses that have ended
  stage: StageId
  switches: { stage: StageId; at: number }[] // when each stage was entered, in order; the first is the start
}

export function newSession(totalMs: number, now: number): Session {
  return { totalMs, startedAt: now, pausedAt: null, pauses: [], stage: 'requirements', switches: [{ stage: 'requirements', at: now }] }
}

const overlap = (ivs: Interval[], from: number, to: number): number => ivs.reduce((sum, v) => sum + Math.max(0, Math.min(v.to, to) - Math.max(v.from, from)), 0)
const allPauses = (s: Session, now: number): Interval[] => (s.pausedAt === null ? s.pauses : [...s.pauses, { from: s.pausedAt, to: now }])

/** Active time: wall time since the start minus every pause. */
export function elapsedMs(s: Session, now: number): number {
  return Math.max(0, now - s.startedAt - overlap(allPauses(s, now), s.startedAt, now))
}

export const remainingMs = (s: Session, now: number): number => Math.max(0, s.totalMs - elapsedMs(s, now))

export function pause(s: Session, now: number): Session {
  return s.pausedAt === null ? { ...s, pausedAt: now } : s
}

export function resume(s: Session, now: number): Session {
  return s.pausedAt === null ? s : { ...s, pausedAt: null, pauses: [...s.pauses, { from: s.pausedAt, to: now }] }
}

export function switchStage(s: Session, stage: StageId, now: number): Session {
  return stage === s.stage ? s : { ...s, stage, switches: [...s.switches, { stage, at: now }] }
}

/** Active time spent in each stage (a stage can be visited several times); pauses are left out. */
export function stageTimes(s: Session, now: number): Record<StageId, number> {
  const out = Object.fromEntries(STAGES.map((x) => [x.id, 0])) as Record<StageId, number>
  const pauses = allPauses(s, now)
  const edges = [...s.switches.map((x) => x.at), now]
  s.switches.forEach((sw, i) => {
    out[sw.stage] += Math.max(0, edges[i + 1] - edges[i] - overlap(pauses, edges[i], edges[i + 1]))
  })
  return out
}

export type Mark = 'half' | 'tenLeft' | 'fiveLeft' | 'oneLeft' | 'over'

/** The announcement marks whose moment falls in (prev, now] of active time. Marks that would fall before the start are skipped. */
export function crossedMarks(s: Session, prevElapsed: number, nowElapsed: number): Mark[] {
  const at: [Mark, number][] = [['half', s.totalMs / 2], ['tenLeft', s.totalMs - 10 * 60_000], ['fiveLeft', s.totalMs - 5 * 60_000], ['oneLeft', s.totalMs - 60_000], ['over', s.totalMs]]
  return at.filter(([, t]) => t > 0 && t > prevElapsed && t <= nowElapsed).map(([m]) => m)
}
