import type { Finding } from '../api/types'

/** The design lint's warning that a per-client guarantee cannot be judged here, for a challenge that has a goal on them. */
export function guaranteeNote(findings: Finding[], maxSessionViolations: number | undefined): string | null {
  if (maxSessionViolations === undefined) return null
  const f = findings.find((x) => x.rule === 'guarantee-not-judged')
  return f ? `${f.message} ${f.fix}` : null
}
