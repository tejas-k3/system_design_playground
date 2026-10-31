import type { Diagram } from '../api/types'

/** The kinds of every component the given node can reach by following connections. */
function reachableKinds(d: Diagram, from: string): Set<string> {
  const kind = new Map(d.nodes.map((n) => [n.id, n.kind]))
  const seen = new Set([from])
  const queue = [from]
  const kinds = new Set<string>()
  for (let id = queue.pop(); id !== undefined; id = queue.pop()) {
    for (const e of d.edges) {
      if (e.from === id && !seen.has(e.to)) {
        seen.add(e.to)
        queue.push(e.to)
        const k = kind.get(e.to)
        if (k) kinds.add(k)
      }
    }
  }
  return kinds
}

/**
 * True when a client asks for writes-follow-reads but reaches only a plain database: that guarantee is judged against
 * a key-value store's copies, so its count would stay 0 and look like a pass. The design lint warns about the same shape.
 */
export function guaranteeNotJudged(d: Diagram): boolean {
  return d.nodes.some((n) => {
    if (n.kind !== 'client' || n.params.consistency !== 'writesFollowReads') return false
    const kinds = reachableKinds(d, n.id)
    return !kinds.has('kvStore') && (kinds.has('database') || kinds.has('readReplica'))
  })
}
