import type { Diagram, StarterStep } from '../api/types'

export type { StarterStep }

export interface StarterProgress extends StarterStep {
  done: boolean
}

/** Every node reached by following connections from the given ones (the starting nodes themselves are not counted). */
function downstream(d: Diagram, from: Set<string>): Set<string> {
  const reached = new Set<string>()
  const queue = [...from]
  for (let id = queue.pop(); id !== undefined; id = queue.pop()) {
    for (const e of d.edges) {
      if (e.from === id && !reached.has(e.to)) {
        reached.add(e.to)
        queue.push(e.to)
      }
    }
  }
  return reached
}

/**
 * The steps form a chain: the first is met by the users already on the canvas, and each later one is met by a
 * component of its kind that is reached, through any connections, from what met the step above it. A database wired
 * straight to the users, or on a separate path from the app server, therefore does not count.
 */
export function checklistProgress(d: Diagram, steps: StarterStep[]): StarterProgress[] {
  const nodes = new Map(d.nodes.map((n) => [n.id, n.kind]))
  let met = new Set<string>()
  return steps.map((s, i) => {
    const pool = i === 0 ? new Set(nodes.keys()) : downstream(d, met)
    met = new Set([...pool].filter((id) => nodes.get(id) === s.kind))
    return { ...s, done: met.size > 0 }
  })
}
