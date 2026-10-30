import type { Diagram, StarterStep } from '../api/types'

export type { StarterStep }

export interface StarterProgress extends StarterStep {
  done: boolean
}

/** A step is done once a component of its kind sits on the canvas and the users can reach it by following connections. */
export function checklistProgress(d: Diagram, steps: StarterStep[]): StarterProgress[] {
  const kindOf = new Map(d.nodes.map((n) => [n.id, n.kind]))
  const seen = new Set(d.nodes.filter((n) => n.kind === 'client').map((n) => n.id))
  const queue = [...seen]
  for (let id = queue.pop(); id !== undefined; id = queue.pop()) {
    for (const e of d.edges) {
      if (e.from === id && kindOf.has(e.to) && !seen.has(e.to)) {
        seen.add(e.to)
        queue.push(e.to)
      }
    }
  }
  const reached = new Set([...seen].map((id) => kindOf.get(id)))
  return steps.map((s) => ({ ...s, done: reached.has(s.kind) }))
}
