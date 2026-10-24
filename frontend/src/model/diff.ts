import type { Diagram, Edge, Node, ParamSpec, ParamValue } from '../api/types'
import type { Reg } from './diagram'

export interface Change {
  nodeId: string // the reference's id of the component
  node: string // its name on your canvas
  setting: string
  mine: string
  reference: string
}
export interface DiagramDiff {
  addedNodes: Node[] // in the reference, with nothing of yours matching
  removedNodes: Node[] // in yours, with nothing in the reference matching
  addedEdges: Edge[] // reference ids
  removedEdges: Edge[] // your ids
  changed: Change[]
  names: { mine: Record<string, string>; ref: Record<string, string> } // what to call each component in a sentence: the name it has on your canvas
  sameArchitecture: boolean // every component and connection matches: only settings (if any) differ
}

function show(spec: ParamSpec, kind: string, v: ParamValue | undefined, reg: Reg): string {
  if (spec.type === 'boolean') return v ? 'on' : 'off'
  if (spec.type === 'select') return reg.optionLabel(kind, spec.key, v) ?? String(v)
  return String(v)
}

/**
 * Pair each reference component with the component of yours that plays the same part. Ids are only the first clue:
 * a design built by hand never uses the reference's ids, so after equal ids the pairs are found by what a component
 * is connected to (components of the same kind with the most already-paired neighbours in common), then by label,
 * then by order.
 */
export function matchNodes(mine: Diagram, ref: Diagram): Map<string, string> {
  const pair = new Map<string, string>() // reference id -> your id
  const taken = new Set<string>()
  const link = (r: string, m: string) => {
    pair.set(r, m)
    taken.add(m)
  }
  const mineById = new Map(mine.nodes.map((n) => [n.id, n]))
  for (const r of ref.nodes) {
    const m = mineById.get(r.id)
    if (m && m.kind === r.kind) link(r.id, m.id)
  }
  const around = (d: Diagram, id: string) => ({
    out: d.edges.filter((e) => e.from === id).map((e) => e.to),
    inn: d.edges.filter((e) => e.to === id).map((e) => e.from),
  })
  for (;;) {
    let best: { r: string; m: string; score: number } | null = null
    for (const r of ref.nodes) {
      if (pair.has(r.id)) continue
      const ra = around(ref, r.id)
      for (const m of mine.nodes) {
        if (taken.has(m.id) || m.kind !== r.kind) continue
        const ma = around(mine, m.id)
        const score =
          ra.out.filter((x) => pair.has(x) && ma.out.includes(pair.get(x)!)).length +
          ra.inn.filter((x) => pair.has(x) && ma.inn.includes(pair.get(x)!)).length
        if (score > 0 && (!best || score > best.score)) best = { r: r.id, m: m.id, score }
      }
    }
    if (!best) break
    link(best.r, best.m)
  }
  for (const r of ref.nodes) { // what is left: same kind and label, then same kind in order
    if (pair.has(r.id)) continue
    const free = mine.nodes.filter((m) => !taken.has(m.id) && m.kind === r.kind)
    const m = free.find((x) => x.label === r.label) ?? free[0]
    if (m) link(r.id, m.id)
  }
  return pair
}

/** Differences between your design and a reference. Components are paired by role (see matchNodes), defaults count as set. */
export function diffDiagrams(mine: Diagram, ref: Diagram, reg: Reg): DiagramDiff {
  const pair = matchNodes(mine, ref) // reference id -> your id
  const back = new Map([...pair].map(([r, m]) => [m, r])) // your id -> reference id
  const mineById = new Map(mine.nodes.map((n) => [n.id, n]))
  const changed: Change[] = []
  for (const r of ref.nodes) {
    const m = mineById.get(pair.get(r.id) ?? '')
    if (!m) continue
    const mp = { ...reg.defaults(m.kind), ...m.params }
    const rp = { ...reg.defaults(r.kind), ...r.params }
    for (const spec of reg.params(r.kind)) {
      if (mp[spec.key] !== rp[spec.key]) {
        changed.push({ nodeId: r.id, node: m.label, setting: spec.label, mine: show(spec, m.kind, mp[spec.key], reg), reference: show(spec, r.kind, rp[spec.key], reg) })
      }
    }
  }
  const mineHas = (a: string, b: string) => mine.edges.some((e) => e.from === a && e.to === b)
  const refHas = (a: string, b: string) => ref.edges.some((e) => e.from === a && e.to === b)
  const addedEdges = ref.edges.filter((e) => {
    const a = pair.get(e.from)
    const b = pair.get(e.to)
    return !(a && b && mineHas(a, b))
  })
  const removedEdges = mine.edges.filter((e) => {
    const a = back.get(e.from)
    const b = back.get(e.to)
    return !(a && b && refHas(a, b))
  })
  const addedNodes = ref.nodes.filter((n) => !pair.has(n.id))
  const removedNodes = mine.nodes.filter((n) => !back.has(n.id))

  // names: yours as they are on your canvas; reference-only components get the first free numbered label of their own
  const names = { mine: {} as Record<string, string>, ref: {} as Record<string, string> }
  const used = new Set(mine.nodes.map((n) => n.label))
  for (const n of mine.nodes) names.mine[n.id] = n.label
  for (const r of ref.nodes) {
    const m = pair.get(r.id)
    if (m) {
      names.ref[r.id] = names.mine[m]
      continue
    }
    let name = r.label
    for (let i = 2; used.has(name); i++) name = `${r.label} ${i}`
    used.add(name)
    names.ref[r.id] = name
  }
  return { addedNodes, removedNodes, addedEdges, removedEdges, changed, names, sameArchitecture: !addedNodes.length && !removedNodes.length && !addedEdges.length && !removedEdges.length }
}

/** One plain sentence per difference, naming components as they are named on your canvas. */
export function describeDiff(d: DiagramDiff, reg: Reg): string[] {
  const kindName = (n: Node) => reg.spec(n.kind)?.name ?? n.kind
  const mine = (id: string) => d.names.mine[id] ?? id
  const ref = (id: string) => d.names.ref[id] ?? id
  return [
    ...d.addedNodes.map((n) => `Add ${ref(n.id)} (${kindName(n)})`),
    ...d.removedNodes.map((n) => `Remove ${mine(n.id)} (${kindName(n)})`),
    ...d.addedEdges.map((e) => `Connect ${ref(e.from)} to ${ref(e.to)}`),
    ...d.removedEdges.map((e) => `Remove the connection from ${mine(e.from)} to ${mine(e.to)}`),
    ...d.changed.map((c) => `${c.node}: ${c.setting} is ${c.mine}, the reference uses ${c.reference}`),
  ]
}
