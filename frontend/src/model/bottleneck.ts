import type { BottleneckPoint, Diagram } from '../api/types'

export interface ComponentOption {
  id: string
  text: string
}

/** The parts a learner can name as the first to run out: everything but the users, told apart when they share a name. */
export function componentOptions(d: Diagram): ComponentOption[] {
  const parts = d.nodes.filter((n) => n.kind !== 'client')
  const shared = new Set(parts.map((n) => n.label).filter((l, i, all) => all.indexOf(l) !== i))
  return parts.map((n) => ({ id: n.id, text: shared.has(n.label) ? `${n.label} (${n.id})` : n.label }))
}

/** The load ladder, with the row where something first runs out marked. */
export function rowsWithFirst(points: BottleneckPoint[], at: number): (BottleneckPoint & { first: boolean })[] {
  return points.map((p, i) => ({ ...p, first: i === at }))
}

export const NONE = '__none__'

/** The guess if it still names a part of the design (or "none"); an empty string once that part has been removed. */
export function usableGuess(d: Diagram, guess: string): string {
  return guess === NONE || componentOptions(d).some((o) => o.id === guess) ? guess : ''
}
