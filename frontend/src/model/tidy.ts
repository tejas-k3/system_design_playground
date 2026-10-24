import type { Diagram, Group, Node, Shape } from '../api/types'
import type { Rect } from '../draw/geometry'
import { type Size, autoLayout } from './arrange'
import { GROUP_HEADER, clampGroupSize, groupRect, groupsOf } from './groups'

/** Room kept between a box's edge and what it holds (the top also clears the header). */
export const BOX_PAD = 24
const BOX_TOP = GROUP_HEADER + 16

const area = (g: Group) => g.w * g.h
const centreIn = (o: Rect, r: Rect) => r.x + r.w / 2 >= o.x && r.x + r.w / 2 <= o.x + o.w && r.y + r.h / 2 >= o.y && r.y + r.h / 2 <= o.y + o.h
const wholly = (o: Rect, r: Rect) => r.x >= o.x && r.y >= o.y && r.x + r.w <= o.x + o.w && r.y + r.h <= o.y + o.h

interface Block {
  size: { w: number; h: number }
  /** positions relative to the block's top-left corner */
  nodes: Map<string, { x: number; y: number }>
  groups: Map<string, { x: number; y: number; w: number; h: number }>
}

/**
 * Tidy up layout that respects boxes. Each box is laid out on its own (its members, and any smaller boxes in it),
 * wrapped to fit them with padding and a header, and then treated as one block in the layout of the box around it.
 * So a box always surrounds exactly the components it held, never cuts through another one, and notes inside a box
 * travel with it. Without boxes this is the plain layered auto layout.
 */
export function tidyDiagram(d: Diagram, size: Size): Diagram {
  return carryShapes(d, tidyLayout(d, size), size)
}

/**
 * Annotation shapes follow what they mark: an ellipse moves with the components it encloses, and each end of an arrow
 * moves with the component (or else the box) it touches. A shape that marks nothing stays where it is.
 */
function carryShapes(before: Diagram, after: Diagram, size: Size): Diagram {
  if (!before.shapes?.length) return after
  const was = new Map(before.nodes.map((n) => [n.id, { x: n.x, y: n.y, ...size(n) }]))
  const shift = new Map<string, { dx: number; dy: number }>()
  for (const n of after.nodes) shift.set(n.id, { dx: n.x - was.get(n.id)!.x, dy: n.y - was.get(n.id)!.y })
  const boxWas = new Map(groupsOf(before).map((g) => [g.id, groupRect(g)]))
  for (const g of groupsOf(after)) {
    const o = boxWas.get(g.id)!
    shift.set(g.id, { dx: g.x - o.x, dy: g.y - o.y })
  }
  const none = { dx: 0, dy: 0 }
  const at = (p: { x: number; y: number }) => {
    const pad = 30
    let best: { id: string; d: number } | null = null
    for (const [id, r] of was) {
      if (p.x < r.x - pad || p.x > r.x + r.w + pad || p.y < r.y - pad || p.y > r.y + r.h + pad) continue
      const dist = Math.hypot(p.x - (r.x + r.w / 2), p.y - (r.y + r.h / 2))
      if (!best || dist < best.d) best = { id, d: dist }
    }
    if (best) return shift.get(best.id) ?? none
    let box: { id: string; a: number } | null = null
    for (const [id, r] of boxWas) if (p.x >= r.x && p.x <= r.x + r.w && p.y >= r.y && p.y <= r.y + r.h && (!box || r.w * r.h < box.a)) box = { id, a: r.w * r.h }
    return box ? shift.get(box.id) ?? none : none
  }
  const shapes = before.shapes.map((s): Shape => {
    if (s.kind === 'arrow') {
      const da = at({ x: s.x, y: s.y })
      const db = at({ x: s.x + s.w, y: s.y + s.h })
      return { ...s, x: s.x + da.dx, y: s.y + da.dy, w: s.w + db.dx - da.dx, h: s.h + db.dy - da.dy }
    }
    const cx = s.x + s.w / 2
    const cy = s.y + s.h / 2
    const held = [...was].filter(([, r]) => Math.hypot((r.x + r.w / 2 - cx) / (s.w / 2), (r.y + r.h / 2 - cy) / (s.h / 2)) <= 1)
    const d = held.length ? { dx: held.reduce((t, [id]) => t + shift.get(id)!.dx, 0) / held.length, dy: held.reduce((t, [id]) => t + shift.get(id)!.dy, 0) / held.length } : at({ x: cx, y: cy })
    return { ...s, x: s.x + d.dx, y: s.y + d.dy }
  })
  return { ...after, shapes }
}

function tidyLayout(d: Diagram, size: Size): Diagram {
  const groups = groupsOf(d)
  if (groups.length === 0) return autoLayout(d, size)

  const nodeBox = (n: Node): Rect => ({ x: n.x, y: n.y, ...size(n) })
  // innermost box of each component, and the box around each box (the smallest one wholly containing it)
  const nodeGroup = new Map<string, string>()
  for (const n of d.nodes) {
    const g = groups.filter((x) => centreIn(groupRect(x), nodeBox(n))).sort((a, b) => area(a) - area(b))[0]
    if (g) nodeGroup.set(n.id, g.id)
  }
  const parent = new Map<string, string>()
  for (const g of groups) {
    const p = groups.filter((o) => o.id !== g.id && area(o) > area(g) && wholly(groupRect(o), groupRect(g))).sort((a, b) => area(a) - area(b))[0]
    if (p) parent.set(g.id, p.id)
  }
  const noteGroup = new Map<string, string>()
  for (const t of d.notes) {
    const g = groups.filter((o) => t.x >= o.x && t.x <= o.x + o.w && t.y >= o.y && t.y <= o.y + o.h).sort((a, b) => area(a) - area(b))[0]
    if (g) noteGroup.set(t.id, g.id)
  }
  // box ids from innermost to outermost, for a component or a box
  const chain = (start: string | undefined): string[] => {
    const out: string[] = []
    for (let g = start; g; g = parent.get(g)) out.push(g)
    return out
  }

  const blocks = new Map<string, Block>()
  const layoutBlock = (boxId: string | null): Block => {
    const childBoxes = groups.filter((g) => (parent.get(g.id) ?? null) === boxId)
    const kids: Block[] = childBoxes.map((g) => layoutBlock(g.id))
    const direct = d.nodes.filter((n) => (nodeGroup.get(n.id) ?? null) === boxId)
    // items: direct components and child boxes, as stand-in nodes for the plain layout
    const itemSize = new Map<string, { w: number; h: number }>()
    const items: Node[] = []
    for (const n of direct) (itemSize.set(n.id, size(n)), items.push({ ...n }))
    childBoxes.forEach((g, i) => {
      itemSize.set(g.id, kids[i].size)
      items.push({ id: g.id, kind: '', label: '', params: {}, x: g.x, y: g.y })
    })
    const itemOf = (nodeId: string): string | undefined => {
      const c = chain(nodeGroup.get(nodeId))
      if (boxId === null) return c.length ? c[c.length - 1] : nodeId
      const at = c.indexOf(boxId)
      if (at < 0) return undefined
      return at === 0 ? nodeId : c[at - 1]
    }
    const edges = new Map<string, { id: string; from: string; to: string; latencyMs: number }>()
    for (const e of d.edges) {
      const a = itemOf(e.from)
      const b = itemOf(e.to)
      if (a && b && a !== b && !edges.has(`${a}>${b}`)) edges.set(`${a}>${b}`, { id: `${a}>${b}`, from: a, to: b, latencyMs: 0 })
    }
    const laid = items.length ? autoLayout({ nodes: items, edges: [...edges.values()], notes: [] }, (n) => itemSize.get(n.id)!) : { nodes: [] as Node[] }
    const minX = Math.min(...laid.nodes.map((n) => n.x))
    const minY = Math.min(...laid.nodes.map((n) => n.y))
    const padX = boxId === null ? 0 : BOX_PAD
    const padTop = boxId === null ? 0 : BOX_TOP
    const padBottom = boxId === null ? 0 : BOX_PAD
    const block: Block = { size: { w: 0, h: 0 }, nodes: new Map(), groups: new Map() }
    let maxX = 0
    let maxY = 0
    for (const it of laid.nodes) {
      const x = it.x - minX + padX
      const y = it.y - minY + padTop
      const s = itemSize.get(it.id)!
      maxX = Math.max(maxX, x + s.w)
      maxY = Math.max(maxY, y + s.h)
      const kid = blocks.get(it.id)
      if (kid) {
        block.groups.set(it.id, { x, y, ...kid.size })
        for (const [id, p] of kid.nodes) block.nodes.set(id, { x: x + p.x, y: y + p.y })
        for (const [id, p] of kid.groups) block.groups.set(id, { ...p, x: x + p.x, y: y + p.y })
      } else block.nodes.set(it.id, { x, y })
    }
    if (boxId === null) block.size = { w: maxX, h: maxY }
    else {
      const g = groups.find((x) => x.id === boxId)!
      // an empty box keeps the size it has
      const empty = items.length === 0
      block.size = empty ? { w: g.w, h: g.h } : { w: clampGroupSize(maxX + padX), h: clampGroupSize(maxY + padBottom) }
    }
    blocks.set(boxId ?? '', block)
    return block
  }
  // children must be in `blocks` before the parent reads them: layoutBlock registers each block as it finishes
  const root = layoutBlock(null)

  const groupShift = new Map<string, { dx: number; dy: number }>()
  const outGroups = groups.map((g) => {
    const p = root.groups.get(g.id)!
    groupShift.set(g.id, { dx: p.x - g.x, dy: p.y - g.y })
    return { ...g, x: p.x, y: p.y, w: p.w, h: p.h }
  })
  const nodes = d.nodes.map((n) => ({ ...n, ...root.nodes.get(n.id)! }))
  const notes = d.notes.map((t) => {
    const s = groupShift.get(noteGroup.get(t.id) ?? '')
    return s ? { ...t, x: t.x + s.dx, y: t.y + s.dy } : t
  })
  return { ...d, nodes, notes, groups: outGroups }
}
