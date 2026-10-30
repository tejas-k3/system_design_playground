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
