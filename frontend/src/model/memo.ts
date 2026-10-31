import type { Finding, Score } from '../api/types'

export interface Memo { design: string; cost: string; watch: string }
export const EMPTY_MEMO: Memo = { design: '', cost: '', watch: '' }

const key = (id: string) => `sdp.memo.${id}`

/** A memo kept in this browser only; a blocked or empty store gives an empty memo. */
export function loadMemo(id: string, storage?: Storage): Memo {
  try {
    const raw = (storage ?? window.localStorage).getItem(key(id))
    if (!raw) return EMPTY_MEMO
    const m = JSON.parse(raw) as Partial<Memo>
    return { design: String(m.design ?? ''), cost: String(m.cost ?? ''), watch: String(m.watch ?? '') }
  } catch {
    return EMPTY_MEMO
  }
}

/** False when the browser would not keep it: the memo then lasts only as long as the page. */
export function saveMemo(id: string, memo: Memo, storage?: Storage): boolean {
  try {
    ;(storage ?? window.localStorage).setItem(key(id), JSON.stringify(memo))
    return true
  } catch {
    return false
  }
}

/** Dollars per thousand requests: the hourly cost over the thousands of requests the run saw per hour. */
export function costPerThousand(score: Score, seconds: number): number | null {
  if (score.run.resolved <= 0 || seconds <= 0) return null
  const perHour = (score.run.resolved / seconds) * 3600
  return score.costPerHour / (perHour / 1000)
}

const pct = (v: number) => `${(v * 100).toFixed(1)}%`

/** What the run actually showed, in plain lines: cost, speed, success, and the failure behaviours that happened. */
export function runEvidence(score: Score, seconds: number): string[] {
  const r = score.run
  const out: string[] = []
  const per = costPerThousand(score, seconds)
  out.push(`Cost: $${score.costPerHour.toFixed(2)} an hour${per === null ? '' : `, about $${per < 0.01 ? per.toFixed(4) : per.toFixed(3)} per 1,000 requests at this run's average load`}.`)
  out.push(`Speed: a p95 of ${Math.round(r.p95)} ms.`)
  if (r.resolved > 0) out.push(`Success: ${pct(r.ok / r.resolved)} of requests succeeded.`)
  const top = Object.entries(r.errors ?? {}).sort((a, b) => b[1] - a[1])[0]
  if (top && top[1] > 0) out.push(`Most failures were ${top[0]} (${top[1].toLocaleString()}).`)
  const facts: [string, number | undefined][] = [
    ['Orders left stuck', r.stuck], ['Orders left half done', r.inconsistent], ['Books: entries that do not balance', r.unbalanced], ['Transactions waiting on a dead coordinator', r.inDoubt],
    ['Acknowledged writes lost', r.lostWrites], ['Requests let through unchecked', r.unchecked], ['Requests that found their user signed out', r.reauth],
    ['Wrong answers accepted', r.wrongAnswers], ['Newer writes lost to a clock', r.clockLost], ['Outputs repeated after a crash', r.repeatedOutputs],
    ['Messages handled out of order', r.jobs?.orderViolations], ['Login sessions lost', r.sessionsLost], ['Double bookings and write conflicts', r.conflicts], ['Duplicate writes', r.duplicates], ['Stale reads', r.staleReads], ['Partial answers', r.partial], ['Write-skew anomalies', r.writeSkew],
  ]
  for (const [what, n] of facts) if (n && n > 0) out.push(`${what}: ${n.toLocaleString()}.`)
  return out
}

export interface OnCall { alerts: string[]; budgetUsedPct: number | null }

/** Alerts the design would raise (its lint warnings and errors) and the share of the error budget the run used. */
export function onCallView(score: Score, findings: Finding[], minSuccessRate?: number): OnCall {
  const alerts = findings.filter((f) => f.severity !== 'info').map((f) => f.message)
  let budgetUsedPct: number | null = null
  if (minSuccessRate !== undefined && score.run.resolved > 0) {
    const failed = 1 - score.run.ok / score.run.resolved
    budgetUsedPct = minSuccessRate >= 1 ? (failed > 0 ? 100 : 0) : Math.round((failed / (1 - minSuccessRate)) * 100)
  }
  return { alerts, budgetUsedPct }
}

/** The memo and the evidence next to it as plain text, for a study partner or a note. */
export interface Line { text: string; sub?: boolean }

export function memoText(title: string, m: Memo, evidence: string[], onCall: Line[]): string {
  const line = (label: string, v: string) => `${label}: ${v.trim() || '(not written)'}`
  return [
    `Trade-off memo: ${title}`, '',
    line('Chosen design', m.design), line('What it costs', m.cost), line('What to monitor', m.watch), '',
    'What the run showed', ...evidence.map((e) => `- ${e}`), '',
    'On call', ...onCall.map((e) => `${e.sub ? '  ' : ''}- ${e.text}`), '',
  ].join('\n')
}
