import type { Diagram, Node } from '../api/types'

export type Size = (n: Node) => { w: number; h: number }
export type AlignMode = 'left' | 'right' | 'top' | 'bottom' | 'centerX' | 'centerY'

/** Lines up the chosen components on one edge or centre line. Needs at least two. */
export function alignNodes(d: Diagram, ids: string[], mode: AlignMode, size: Size): Diagram {
  const pick = d.nodes.filter((n) => ids.includes(n.id))
  if (pick.length < 2) return d
  const left = Math.min(...pick.map((n) => n.x))
  const right = Math.max(...pick.map((n) => n.x + size(n).w))
  const top = Math.min(...pick.map((n) => n.y))
  const bottom = Math.max(...pick.map((n) => n.y + size(n).h))
  const move = (n: Node): Node => {
    const { w, h } = size(n)
    switch (mode) {
      case 'left': return { ...n, x: left }
      case 'right': return { ...n, x: right - w }
      case 'top': return { ...n, y: top }
      case 'bottom': return { ...n, y: bottom - h }
      case 'centerX': return { ...n, x: (left + right) / 2 - w / 2 }
      case 'centerY': return { ...n, y: (top + bottom) / 2 - h / 2 }
    }
  }
  const set = new Set(pick.map((n) => n.id))
  return { ...d, nodes: d.nodes.map((n) => (set.has(n.id) ? move(n) : n)) }
}

/** Spaces the chosen components so the gaps between neighbours are equal; the two outermost stay put. Needs three. */
export function distributeNodes(d: Diagram, ids: string[], axis: 'x' | 'y', size: Size): Diagram {
  const pick = d.nodes.filter((n) => ids.includes(n.id))
  if (pick.length < 3) return d
  const len = (n: Node) => (axis === 'x' ? size(n).w : size(n).h)
  const sorted = [...pick].sort((a, b) => a[axis] - b[axis] || a.id.localeCompare(b.id))
  const first = sorted[0]
  const last = sorted[sorted.length - 1]
  const span = last[axis] + len(last) - first[axis]
  const gap = (span - sorted.reduce((s, n) => s + len(n), 0)) / (sorted.length - 1)
  const at = new Map<string, number>()
  let cursor = first[axis]
  for (const n of sorted) {
    at.set(n.id, cursor)
    cursor += len(n) + gap
  }
  return { ...d, nodes: d.nodes.map((n) => (at.has(n.id) ? { ...n, [axis]: at.get(n.id)! } : n)) }
}

const GAP_X = 90
const GAP_Y = 40

/**
 * A layered left-to-right layout: a component sits one column right of the furthest component that feeds it,
 * and within a column components are ordered by the average height of what feeds them, which keeps connections
 * short and mostly uncrossed. Cycles are tolerated (an edge that closes a loop is ignored). Pure and deterministic;
 * separate pieces of the design are stacked in their own rows. Notes are left where they are.
 */
export function autoLayout(d: Diagram, size: Size, gapX = GAP_X, gapY = GAP_Y): Diagram {
  const ids = d.nodes.map((n) => n.id)
  const index = new Map(ids.map((id, i) => [id, i]))
  const out = new Map<string, string[]>()
  const into = new Map<string, string[]>()
  for (const id of ids) {
    out.set(id, [])
    into.set(id, [])
  }
  // depth-first search to find the edges that close a loop, so they can be set aside
  const state = new Map<string, 0 | 1 | 2>()
  const back = new Set<string>()
  const visit = (id: string) => {
    state.set(id, 1)
    for (const e of d.edges.filter((x) => x.from === id && index.has(x.to))) {
      const s = state.get(e.to) ?? 0
      if (s === 1) back.add(e.id)
      else if (s === 0) visit(e.to)
    }
    state.set(id, 2)
  }
  for (const id of ids) if (!state.get(id)) visit(id)
  for (const e of d.edges) {
    if (back.has(e.id) || !index.has(e.from) || !index.has(e.to) || e.from === e.to) continue
    out.get(e.from)!.push(e.to)
    into.get(e.to)!.push(e.from)
  }
  // column = longest path from a source
  const layer = new Map<string, number>()
  const depth = (id: string): number => {
    if (layer.has(id)) return layer.get(id)!
    const l = into.get(id)!.reduce((m, p) => Math.max(m, depth(p) + 1), 0)
    layer.set(id, l)
    return l
  }
  ids.forEach(depth)
  // separate pieces (weakly connected components) get their own rows
  const comp = new Map<string, number>()
  let nComp = 0
  for (const id of ids) {
    if (comp.has(id)) continue
    const stack = [id]
    comp.set(id, nComp)
    while (stack.length) {
      const cur = stack.pop()!
      for (const nb of [...out.get(cur)!, ...into.get(cur)!]) if (!comp.has(nb)) (comp.set(nb, nComp), stack.push(nb))
    }
    nComp++
  }
  const sizeOf = new Map(d.nodes.map((n) => [n.id, size(n)]))
  const colW = new Map<number, number>()
  for (const id of ids) colW.set(layer.get(id)!, Math.max(colW.get(layer.get(id)!) ?? 0, sizeOf.get(id)!.w))
  const colX = new Map<number, number>()
  let x = 0
  for (const l of [...colW.keys()].sort((a, b) => a - b)) {
    colX.set(l, x)
    x += colW.get(l)! + gapX
  }
  const y = new Map<string, number>()
  let top = 0
  for (let c = 0; c < nComp; c++) {
    const members = ids.filter((id) => comp.get(id) === c)
    const layers = [...new Set(members.map((id) => layer.get(id)!))].sort((a, b) => a - b)
    let height = 0
    for (const l of layers) {
      const col = members.filter((id) => layer.get(id) === l)
      // order by the average position of the parents already placed; ties keep the diagram's own order
      const key = (id: string) => {
        const ps = into.get(id)!.filter((p) => y.has(p))
        return ps.length ? ps.reduce((s, p) => s + y.get(p)!, 0) / ps.length : Infinity
      }
      col.sort((a, b) => key(a) - key(b) || index.get(a)! - index.get(b)!)
      let cy = top
      for (const id of col) {
        y.set(id, cy)
        cy += sizeOf.get(id)!.h + gapY
      }
      height = Math.max(height, cy - top)
    }
    top += height + gapY
  }
  return { ...d, nodes: d.nodes.map((n) => ({ ...n, x: colX.get(layer.get(n.id)!)!, y: y.get(n.id)! })) }
}
