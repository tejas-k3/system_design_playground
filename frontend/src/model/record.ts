// What the browser remembers about HOW a challenge went, not only the stars: wrong predictions, hints, attempts, and
// whether the answer was shown. Pure functions; the store keeps one record per challenge id next to the stars.

export interface ChallengeRecord {
  wrong: number // predictions answered wrong
  hints: number // the highest hint revealed (0 to 3)
  runs: number // runs finished up to and including the first pass; counting stops there
  passed: boolean // the challenge has been passed at least once
  earlyPass: boolean // the first pass came before hint 3 was revealed (meaningful once passed)
  answerUsed: boolean // "Show the answer" was used
}

export const MAX_COUNT = 1000
export const MAX_HINTS = 10

export const emptyRecord = (): ChallengeRecord => ({ wrong: 0, hints: 0, runs: 0, passed: false, earlyPass: false, answerUsed: false })

const bump = (n: number) => Math.min(MAX_COUNT, n + 1)

export function noteWrong(r: ChallengeRecord = emptyRecord()): ChallengeRecord {
  return { ...r, wrong: bump(r.wrong) }
}

/** A hint was revealed; `level` is its number (1 to 3). Only the highest counts. */
export function noteHint(r: ChallengeRecord = emptyRecord(), level: number): ChallengeRecord {
  return level > r.hints ? { ...r, hints: Math.min(MAX_HINTS, Math.floor(level)) } : r
}

/** A run finished. Before the first pass every run counts as an attempt; the first pass also fixes `earlyPass`. */
export function noteRun(r: ChallengeRecord = emptyRecord(), pass: boolean): ChallengeRecord {
  if (r.passed) return r
  return { ...r, runs: bump(r.runs), passed: pass, earlyPass: pass && r.hints < 3 }
}

export function noteAnswer(r: ChallengeRecord = emptyRecord()): ChallengeRecord {
  return r.answerUsed ? r : { ...r, answerUsed: true }
}

/** The review rule: a passed challenge needs review when the answer was shown or a prediction was wrong. */
export function needsReview(r: ChallengeRecord | undefined): boolean {
  return !!r && r.passed && (r.answerUsed || r.wrong > 0)
}

export const REVIEW_EXPLAIN = 'Passed, but the answer was shown or a prediction was wrong on at least one challenge here. Try it again without the answer.'

/** A plain sentence for one challenge, for the skill map's description. */
export function reviewReason(r: ChallengeRecord): string {
  const parts: string[] = []
  if (r.answerUsed) parts.push('the answer was shown')
  if (r.wrong > 0) parts.push(`${r.wrong} ${r.wrong === 1 ? 'prediction was' : 'predictions were'} wrong`)
  return parts.join(' and ')
}
