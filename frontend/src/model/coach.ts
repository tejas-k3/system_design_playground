import type { Diagram, Stats } from '../api/types'

export interface Insight {
  rule: string
  severity: 'error' | 'warn' | 'info'
  nodeId: string
  title: string
  detail: string
}

const MAX = 5
const RANK = { error: 0, warn: 1, info: 2 }

/**
 * Plain-language explanations of what is limiting the design right now, from live stats only.
 * Conservative on purpose: each rule needs several signals to agree, and the list is short.
 */
export function coach(d: Diagram, stats: Stats | undefined): Insight[] {
  if (!stats || !stats.nodes) return []
  const out: Insight[] = []
  const label = (id: string) => d.nodes.find((n) => n.id === id)?.label ?? id
  const downstream = (id: string) => d.edges.filter((e) => e.from === id).map((e) => stats.nodes[e.to]).filter(Boolean)
  const add = (i: Insight) => out.push(i)

  for (const n of d.nodes) {
    const s = stats.nodes[n.id]
    if (!s) continue
    const name = label(n.id)
    let blamed = false

    if (n.kind === 'appServer' && s.util >= 0.9 && s.qLen > 0) {
      const down = downstream(n.id)
      if (down.some((x) => x.down || x.cut) && Number(n.params?.callTimeoutMs ?? 0) === 0) {
        blamed = true
        add({ rule: 'stuck-on-dependency', severity: 'error', nodeId: n.id, title: `${name}: stuck waiting on a dependency that is not answering`,
          detail: `${name}'s threads are all waiting on something that is down or cut off, and it has no call timeout, so they never give up. Set a call timeout so they fail fast and free up.` })
      } else if (down.some((x) => x.down || x.cut)) {
        blamed = true
      } else if (down.length > 0 && down.every((x) => x.util < 0.3)) {
        blamed = true
        add({ rule: 'threads-bottleneck', severity: 'warn', nodeId: n.id, title: `${name}: its threads are the bottleneck`,
          detail: `${name} has every thread busy and requests waiting, while everything behind it is mostly idle. More threads or more servers would help; a faster database would not.` })
      }
    }
    if (!blamed && s.cap > 0 && s.busy >= s.cap && s.qLen > 0 && s.arr > 0 && s.err / s.arr > 0.05) {
      add({ rule: 'queue-full', severity: 'error', nodeId: n.id, title: `${name}: queue full, turning requests away`,
        detail: `${name} is out of threads and its queue is full, so about ${Math.round((100 * s.err) / s.arr)}% of requests are refused. Raise capacity, shorten how long work may wait, or shed load earlier.` })
    }
    if (s.phase) {
      add({ rule: 'failover', severity: 'warn', nodeId: n.id, title: `${name}: failing over`,
        detail: `${name} is ${s.phase === 'detecting' ? 'not yet noticed as down' : 'being replaced by a replica'}, about ${Math.max(1, Math.ceil(s.phaseLeftSec ?? 0))} s to go. Writes fail until the new primary is ready.` })
    } else if ((s.reasons?.noPrimary ?? 0) > 0) {
      add({ rule: 'failover', severity: 'error', nodeId: n.id, title: `${name}: writes refused, no primary`,
        detail: `${name} has no primary to accept writes. Set it to promote a replica when it fails.` })
    } else if (s.down || s.cut) {
      add({ rule: 'node-down', severity: 'error', nodeId: n.id, title: `${name} is ${s.down ? 'down' : 'cut off from the network'}`,
        detail: `Requests that need ${name} fail or wait. Look at timeouts, retries and whether another copy can take over.` })
    }
    const max = Number(n.params?.maxInstances ?? 8)
    if (n.kind === 'appServer' && n.params?.autoscale && (s.instances ?? 0) >= max && s.util > Number(n.params?.targetUtilPct ?? 60) / 100) {
      add({ rule: 'autoscale-ceiling', severity: 'warn', nodeId: n.id, title: `${name}: autoscaler at its ceiling`,
        detail: `All ${max} servers are running and still busier than the target. Raise the maximum, or reduce the work each request does.` })
    }
    if (n.kind === 'cache' && s.hitRate >= 0 && s.hitRate < 0.3 && downstream(n.id).some((x) => x.util > 0.8)) {
      add({ rule: 'cache-ineffective', severity: 'warn', nodeId: n.id, title: `${name}: not absorbing the load`,
        detail: `Only ${Math.round(s.hitRate * 100)}% of lookups hit while the database behind it is struggling. The cache may be too small, too cold, or expiring entries too fast.` })
    }
  }
  for (const e of d.edges) {
    if (stats.edges?.[e.id]?.breaker === 'open') {
      add({ rule: 'breaker-open', severity: 'error', nodeId: e.from, title: `${label(e.from)}: circuit breaker open`,
        detail: `${label(e.from)} has stopped calling ${label(e.to)} for a few seconds after repeated failures. Requests that need it fail fast until it recovers.` })
    }
  }
  return out.sort((a, b) => RANK[a.severity] - RANK[b.severity]).slice(0, MAX)
}
