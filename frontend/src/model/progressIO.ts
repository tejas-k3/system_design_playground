import { MAX_COUNT, MAX_HINTS, emptyRecord, type ChallengeRecord } from './record'

// Export and import of the local progress as a JSON file. The imported file is untrusted: it is parsed, size-capped and
// rebuilt field by field into fresh objects. Unknown fields are dropped, bad entries are skipped, anything that is not
// the expected shape is an error.

export const FORMAT = 'sdp-progress'
export const MAX_FILE_BYTES = 200_000
export const MAX_ENTRIES = 500
const ID = /^[a-z0-9][a-z0-9_-]{0,63}$/
const FORBIDDEN = new Set(['__proto__', 'constructor', 'prototype', 'toString', 'valueOf', 'hasOwnProperty'])

export interface Progress {
  progress: Record<string, number>
  records: Record<string, ChallengeRecord>
}

export function exportProgress(p: Progress): string {
  return JSON.stringify({ format: FORMAT, version: 1, progress: p.progress, records: p.records }, null, 2)
}

export type ImportResult = ({ ok: true; skipped: number } & Progress) | { ok: false; error: string }

const isObj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)
const int = (v: unknown, max: number): number | null => (typeof v === 'number' && Number.isInteger(v) && v >= 0 && v <= max ? v : null)
const goodId = (k: string) => ID.test(k) && !FORBIDDEN.has(k)

function record(v: unknown): ChallengeRecord | null {
  if (!isObj(v)) return null
  const wrong = int(v.wrong, MAX_COUNT)
  const hints = int(v.hints, MAX_HINTS)
  const runs = int(v.runs, MAX_COUNT)
  if (wrong === null || hints === null || runs === null) return null
  if (typeof v.passed !== 'boolean' || typeof v.earlyPass !== 'boolean' || typeof v.answerUsed !== 'boolean') return null
  return { ...emptyRecord(), wrong, hints, runs, passed: v.passed, earlyPass: v.passed && v.earlyPass, answerUsed: v.answerUsed }
}

export function parseImport(text: string): ImportResult {
  if (typeof text !== 'string' || text.length > MAX_FILE_BYTES) return { ok: false, error: 'That file is too large to be a progress export.' }
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    return { ok: false, error: 'That file is not valid JSON.' }
  }
  if (!isObj(raw) || raw.format !== FORMAT || raw.version !== 1) return { ok: false, error: 'That is not a progress export from this app.' }
  if (!isObj(raw.progress)) return { ok: false, error: 'The progress section is missing or malformed.' }
  if (raw.records !== undefined && !isObj(raw.records)) return { ok: false, error: 'The records section is malformed.' }
  const stars = Object.entries(raw.progress)
  const recs = Object.entries(raw.records ?? {})
  if (stars.length > MAX_ENTRIES || recs.length > MAX_ENTRIES) return { ok: false, error: 'That file has too many entries.' }
  const progress: Record<string, number> = {}
  const records: Record<string, ChallengeRecord> = {}
  let skipped = 0
  for (const [k, v] of stars) {
    const n = int(v, 3)
    if (goodId(k) && n !== null) progress[k] = n
    else skipped++
  }
  for (const [k, v] of recs) {
    const r = record(v)
    if (goodId(k) && r) records[k] = r
    else skipped++
  }
  return { ok: true, skipped, progress, records }
}

/** Add imported progress to what is here: stars keep the higher value; an imported note replaces the local one for the same challenge. */
export function mergeProgress(local: Progress, imported: Progress): Progress {
  const progress = { ...local.progress }
  for (const [k, v] of Object.entries(imported.progress)) progress[k] = Math.max(progress[k] ?? 0, v)
  return { progress, records: { ...local.records, ...imported.records } }
}
