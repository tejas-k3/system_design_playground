import { STAGES, type Session, type StageId } from './timer'

// What an interview session remembers. It lives only in the browser (localStorage); nothing here is sent anywhere.
// sanitizeInterview is the one gate from stored JSON to this shape: unknown fields are dropped, numbers are clamped,
// lists and strings are capped, and anything that is not recognisably an interview is refused whole.

export const MAX_ROWS = 30
export const MAX_TEXT = 500

export interface Requirement {
  label: string
  target: number | null // a number for the non-functional ones (availability %, p95 ms, RPS, GB), if given
  unit: string
}
export interface Endpoint {
  method: string
  path: string
  request: string
  response: string
  notes: string
}
export interface Table {
  name: string
  fields: string
  key: string
  indexes: string
  access: string
}
export const RUBRIC_AREAS = ['scalability', 'availability', 'consistency', 'cost', 'communication'] as const
export type RubricArea = (typeof RUBRIC_AREAS)[number]

export interface EstimateInputs {
  mau: number
  dauPct: number
  actionsPerDay: number
  peakFactor: number
  readPct: number
  bytesEach: number
  retentionYears: number
  replication: number
  payloadBytes: number
  perServerQps: number
  headroomPct: number
}

export interface InterviewData {
  session: Session | null
  functional: string[]
  nonFunctional: Requirement[]
  estimate: EstimateInputs
  api: Endpoint[]
  tables: Table[]
  rubric: Record<RubricArea, number> // 0 = not rated, 1..5
  notes: string
}

export const defaultEstimate = (): EstimateInputs => ({ mau: 100e6, dauPct: 50, actionsPerDay: 10, peakFactor: 3, readPct: 90, bytesEach: 500, retentionYears: 3, replication: 3, payloadBytes: 10e3, perServerQps: 400, headroomPct: 30 })

export function emptyInterview(): InterviewData {
  return { session: null, functional: [], nonFunctional: [], estimate: defaultEstimate(), api: [], tables: [], rubric: { scalability: 0, availability: 0, consistency: 0, cost: 0, communication: 0 }, notes: '' }
}

const isObj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)
const text = (v: unknown, max = MAX_TEXT): string | null => (typeof v === 'string' ? v.slice(0, max) : null)
const num = (v: unknown, lo: number, hi: number): number | null => (typeof v === 'number' && Number.isFinite(v) ? Math.min(hi, Math.max(lo, v)) : null)

function list<T>(v: unknown, one: (x: unknown) => T | null): T[] | null {
  if (!Array.isArray(v)) return null
  const out: T[] = []
  for (const x of v.slice(0, MAX_ROWS)) {
    const r = one(x)
    if (r === null) return null // a malformed row means the blob is not ours: refuse it whole
    out.push(r)
  }
  return out
}

function session(v: unknown): Session | null | undefined {
  if (v === null || v === undefined) return null
  if (!isObj(v)) return undefined
  const total = num(v.totalMs, 60_000, 6 * 3_600_000)
  const started = num(v.startedAt, 0, 8.64e15)
  const stage = STAGES.find((s) => s.id === v.stage)?.id as StageId | undefined
  const pausedAt = v.pausedAt === null ? null : num(v.pausedAt, 0, 8.64e15)
  const pauses = list(v.pauses, (p) => (isObj(p) && num(p.from, 0, 8.64e15) !== null && num(p.to, 0, 8.64e15) !== null ? { from: p.from as number, to: p.to as number } : null))
  const switches = list(v.switches, (p) => {
    const st = isObj(p) ? (STAGES.find((s) => s.id === p.stage)?.id as StageId | undefined) : undefined
    return isObj(p) && st && num(p.at, 0, 8.64e15) !== null ? { stage: st, at: p.at as number } : null
  })
  if (total === null || started === null || !stage || pauses === null || switches === null || switches.length === 0 || (v.pausedAt !== null && pausedAt === null)) return undefined
  return { totalMs: total, startedAt: started, pausedAt, pauses, stage, switches }
}

/** The interview data from stored JSON, or undefined if it is not one (the rest of the saved state is unaffected). */
export function sanitizeInterview(raw: unknown): InterviewData | undefined {
  if (!isObj(raw)) return undefined
  const sess = session(raw.session)
  const functional = list(raw.functional, (x) => text(x, 200))
  const nonFunctional = list(raw.nonFunctional, (x) => {
    if (!isObj(x)) return null
    const label = text(x.label, 200)
    const unit = text(x.unit, 20)
    const target = x.target === null ? null : num(x.target, 0, 1e15)
    return label === null || unit === null || (x.target !== null && target === null) ? null : { label, target, unit }
  })
  const api = list(raw.api, (x) => {
    if (!isObj(x)) return null
    const f = [text(x.method, 10), text(x.path, 200), text(x.request), text(x.response), text(x.notes)]
    return f.some((s) => s === null) ? null : { method: f[0]!, path: f[1]!, request: f[2]!, response: f[3]!, notes: f[4]! }
  })
  const tables = list(raw.tables, (x) => {
    if (!isObj(x)) return null
    const f = [text(x.name, 100), text(x.fields), text(x.key, 200), text(x.indexes), text(x.access)]
    return f.some((s) => s === null) ? null : { name: f[0]!, fields: f[1]!, key: f[2]!, indexes: f[3]!, access: f[4]! }
  })
  const notes = text(raw.notes, 5000)
  const e = isObj(raw.estimate) ? raw.estimate : null
  const d = defaultEstimate()
  const est = e
    ? {
        mau: num(e.mau, 0, 1e12), dauPct: num(e.dauPct, 0, 100), actionsPerDay: num(e.actionsPerDay, 0, 1e6), peakFactor: num(e.peakFactor, 1, 100), readPct: num(e.readPct, 0, 100),
        bytesEach: num(e.bytesEach, 0, 1e9), retentionYears: num(e.retentionYears, 0, 100), replication: num(e.replication, 1, 10), payloadBytes: num(e.payloadBytes, 0, 1e10),
        perServerQps: num(e.perServerQps, 1, 1e6), headroomPct: num(e.headroomPct, 0, 95),
      }
    : null
  const r = isObj(raw.rubric) ? raw.rubric : null
  if (sess === undefined || !functional || !nonFunctional || !api || !tables || notes === null || !est || !r) return undefined
  if (Object.values(est).some((v) => v === null)) return undefined
  const rubric = {} as Record<RubricArea, number>
  for (const a of RUBRIC_AREAS) {
    const v = num(r[a], 0, 5)
    if (v === null) return undefined
    rubric[a] = Math.round(v)
  }
  return { session: sess, functional, nonFunctional, estimate: { ...d, ...(est as EstimateInputs) }, api, tables, rubric, notes }
}
