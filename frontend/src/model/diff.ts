import type { Diagram, Edge, Node, ParamSpec, ParamValue } from '../api/types'
import type { Reg } from './diagram'

export interface Change {
  nodeId: string
  node: string
  setting: string
  mine: string
  reference: string
}
export interface DiagramDiff {
  addedNodes: Node[] // in the reference, not in yours
  removedNodes: Node[] // in yours, not in the reference
  addedEdges: Edge[]
  removedEdges: Edge[]
  changed: Change[]
  labels: Record<string, string> // every component's label, yours first, so sentences can name both ends of a connection
}

function show(spec: ParamSpec, kind: string, v: ParamValue | undefined, reg: Reg): string {
  if (spec.type === 'boolean') return v ? 'on' : 'off'
  if (spec.type === 'select') return reg.optionLabel(kind, spec.key, v) ?? String(v)
  return String(v)
}

/** Differences between your design and a reference, matched by component id; defaults count as set. */
export function diffDiagrams(mine: Diagram, ref: Diagram, reg: Reg): DiagramDiff {
  const mineNodes = new Map(mine.nodes.map((n) => [n.id, n]))
  const refNodes = new Map(ref.nodes.map((n) => [n.id, n]))
  const mineEdges = new Set(mine.edges.map((e) => e.id))
  const refEdges = new Set(ref.edges.map((e) => e.id))
  const changed: Change[] = []
  for (const r of ref.nodes) {
    const m = mineNodes.get(r.id)
    if (!m || m.kind !== r.kind) continue
    const mp = { ...reg.defaults(m.kind), ...m.params }
    const rp = { ...reg.defaults(r.kind), ...r.params }
    for (const spec of reg.params(r.kind)) {
      if (mp[spec.key] !== rp[spec.key]) {
        changed.push({ nodeId: r.id, node: m.label, setting: spec.label, mine: show(spec, m.kind, mp[spec.key], reg), reference: show(spec, r.kind, rp[spec.key], reg) })
      }
    }
  }
  return {
    addedNodes: ref.nodes.filter((n) => !mineNodes.has(n.id)),
    removedNodes: mine.nodes.filter((n) => !refNodes.has(n.id)),
    addedEdges: ref.edges.filter((e) => !mineEdges.has(e.id)),
    removedEdges: mine.edges.filter((e) => !refEdges.has(e.id)),
    changed,
    labels: Object.fromEntries([...ref.nodes, ...mine.nodes].map((n) => [n.id, n.label])),
  }
}

/** One plain sentence per difference. */
export function describeDiff(d: DiagramDiff, reg: Reg): string[] {
  const counts = new Map<string, number>()
  for (const l of Object.values(d.labels)) counts.set(l, (counts.get(l) ?? 0) + 1)
  const label = (id: string) => {
    const l = d.labels[id] ?? id
    return (counts.get(l) ?? 0) > 1 ? `${l} (${id})` : l
  }
  const kindName = (n: Node) => reg.spec(n.kind)?.name ?? n.kind
  return [
    ...d.addedNodes.map((n) => `Add ${label(n.id)} (${kindName(n)})`),
    ...d.removedNodes.map((n) => `Remove ${label(n.id)} (${kindName(n)})`),
    ...d.addedEdges.map((e) => `Connect ${label(e.from)} to ${label(e.to)}`),
    ...d.removedEdges.map((e) => `Remove the connection from ${label(e.from)} to ${label(e.to)}`),
    ...d.changed.map((c) => `${label(c.nodeId)}: ${c.setting} is ${c.mine}, the reference uses ${c.reference}`),
  ]
}
