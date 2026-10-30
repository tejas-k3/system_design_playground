import type { Alternative } from '../api/types'

/** Pick the first stored counter-question that failed, or the first alternative when all pass. */
export function selectDefendAlternative(alternatives: Alternative[]): number | null {
  if (alternatives.length === 0) return null
  const index = alternatives.findIndex((alternative) => !alternative.pass)
  return index < 0 ? 0 : index
}

export function defendQuestion(label: string): string {
  return `Why not ${label.charAt(0).toLowerCase()}${label.slice(1)}?`
}
