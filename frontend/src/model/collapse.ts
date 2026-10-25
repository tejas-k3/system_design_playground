import type { Diagram, Group, Node } from '../api/types'
import type { Rect } from '../draw/geometry'
import { groupMembers, groupRect, groupsOf } from './groups'
import { shapeRect, shapesOf } from './shapes'

/**
 * Collapsing a box is purely visual. The diagram, and so what the simulation runs, never changes except for the
 * box's `collapsed` flag. This module derives, from the diagram alone, what the canvas should show: which items are
 * hidden, where the compact "chip" of each collapsed box sits, and where connections to hidden members are drawn.
 */
export const CHIP_W = 260
export const CHIP_H = 64
/** The narrowest a chip is drawn, even for a small box. */
const CHIP_MIN_W = 180

/** Components that show a utilisation bar on the canvas (a client or load balancer has none). */
export const hasUtilBar = (kind: string): boolean => kind !== 'client' && kind !== 'loadBalancer'

/** One drawn connection after collapsing: where it starts and ends, and which connections it stands for. */
export interface Route {
  a: Rect
  b: Rect
  /** false for a connection folded into another one (same neighbour, same direction): it is not drawn on its own */
  draw: boolean
  /** ids of every connection this line stands for; the first is the one that is drawn */
  members: string[]
}

export interface CollapseView {
  /** true when at least one box is collapsed */
  any: boolean
  /** the chip of each collapsed box that is not itself inside another collapsed box */
  chips: Map<string, Rect>
  hiddenNodes: Set<string>
  hiddenNotes: Set<string>
  hiddenGroups: Set<string>
  hiddenShapes: Set<string>
  /** hidden component id -> the collapsed box whose chip stands in for it */
  owner: Map<string, string>
  /** every connection id -> its route; null when it joins two members of the same collapsed box. Empty when nothing is collapsed. */
  routes: Map<string, Route | null>
}

const NONE: CollapseView = { any: false, chips: new Map(), hiddenNodes: new Set(), hiddenNotes: new Set(), hiddenGroups: new Set(), hiddenShapes: new Set(), owner: new Map(), routes: new Map() }

export const chipRect = (g: Group): Rect => ({ x: g.x, y: g.y, w: Math.min(CHIP_W, Math.max(CHIP_MIN_W, g.w)), h: CHIP_H })

const wholly = (outer: Rect, r: Rect): boolean => r.x >= outer.x && r.y >= outer.y && r.x + r.w <= outer.x + outer.w && r.y + r.h <= outer.y + outer.h

export function collapseView(d: Diagram, rectOf: (n: Node) => Rect): CollapseView {
  const collapsed = groupsOf(d).filter((g) => g.collapsed)
  if (collapsed.length === 0) return NONE
  const nodeIds = new Set(d.nodes.map((n) => n.id))
  const noteIds = new Set(d.notes.map((n) => n.id))
  const groupIds = new Set(groupsOf(d).map((g) => g.id))
  // everything inside any collapsed box, boxes included; a collapsed box inside another collapsed box is itself hidden
  const members = new Map<string, string[]>()
  const hiddenGroups = new Set<string>()
  for (const g of collapsed) {
    const m = groupMembers(d, g.id, rectOf)
    members.set(g.id, m)
    for (const id of m) if (groupIds.has(id)) hiddenGroups.add(id)
  }
  const chips = new Map<string, Rect>()
  const hiddenNodes = new Set<string>()
  const hiddenNotes = new Set<string>()
  const hiddenShapes = new Set<string>()
  const owner = new Map<string, string>()
  // smallest box first, so a component held by two overlapping boxes is stood in for by the tighter one
  for (const g of [...collapsed].filter((x) => !hiddenGroups.has(x.id)).sort((a, b) => a.w * a.h - b.w * b.h)) {
    chips.set(g.id, chipRect(g))
    for (const id of members.get(g.id) ?? []) {
      if (nodeIds.has(id)) {
        hiddenNodes.add(id)
        if (!owner.has(id)) owner.set(id, g.id)
      } else if (noteIds.has(id)) hiddenNotes.add(id)
    }
    for (const s of shapesOf(d)) if (wholly(groupRect(g), shapeRect(s))) hiddenShapes.add(s.id)
  }
  const nodes = new Map(d.nodes.map((n) => [n.id, n]))
  const routes = new Map<string, Route | null>()
  const first = new Map<string, Route>()
  const end = (id: string): { key: string; rect: Rect } | null => {
    const n = nodes.get(id)
    if (!n) return null
    const o = owner.get(id)
    return o ? { key: `box:${o}`, rect: chips.get(o)! } : { key: id, rect: rectOf(n) }
  }
  for (const e of d.edges) {
    const f = end(e.from)
    const t = end(e.to)
    if (!f || !t || f.key === t.key) {
      routes.set(e.id, null)
      continue
    }
    const hidden = f.key.startsWith('box:') || t.key.startsWith('box:')
    const key = `${f.key}>${t.key}`
    const prior = hidden ? first.get(key) : undefined
    if (prior) {
      prior.members.push(e.id)
      routes.set(e.id, { a: prior.a, b: prior.b, draw: false, members: prior.members })
      continue
    }
    const r: Route = { a: f.rect, b: t.rect, draw: true, members: [e.id] }
    if (hidden) first.set(key, r)
    routes.set(e.id, r)
  }
  return { any: true, chips, hiddenNodes, hiddenNotes, hiddenGroups, hiddenShapes, owner, routes }
}

/** Where a box is drawn: its chip when collapsed, otherwise its full rectangle. */
export function groupDisplayRect(v: CollapseView, g: Group): Rect {
  return v.chips.get(g.id) ?? groupRect(g)
}

/** Is this component, note, box or annotation out of sight behind a collapsed box? */
export const isHidden = (v: CollapseView, id: string): boolean => v.hiddenNodes.has(id) || v.hiddenNotes.has(id) || v.hiddenGroups.has(id) || v.hiddenShapes.has(id)

export const visibleNodes = (d: Diagram, v: CollapseView): Node[] => (v.any ? d.nodes.filter((n) => !v.hiddenNodes.has(n.id)) : d.nodes)

/** Keeps the ids that can be selected on the canvas: not hidden, and (for a connection) actually drawn. */
export function visibleIds(d: Diagram, v: CollapseView, ids: string[]): string[] {
  if (!v.any) return ids
  const edges = new Set(d.edges.map((e) => e.id))
  return ids.filter((id) => !isHidden(v, id) && (!edges.has(id) || v.routes.get(id)?.draw === true))
}

export interface NodeFigures {
  util: number
  arr: number
  err: number
  down?: boolean
}
export interface ChipSummary {
  /** components inside, nested boxes' included */
  count: number
  /** worst utilisation of the members, in whole percent; null before there are numbers */
  busiest: number | null
  /** members that are down */
  down: number
  /** members failing more than 2% of what they receive */
  failing: number
}

/** What a collapsed box says about the components it hides, so a problem inside cannot go unseen. */
export function chipSummary(v: CollapseView, d: Diagram, boxId: string, stats: Record<string, NodeFigures | undefined>): ChipSummary {
  let count = 0
  let busiest: number | null = null
  let down = 0
  let failing = 0
  for (const n of d.nodes) {
    if (v.owner.get(n.id) !== boxId) continue
    count++
    const s = stats[n.id]
    if (!s) continue
    if (s.down) down++
    else if (s.arr > 2 && s.err / Math.max(s.arr, 1) > 0.02) failing++
    if (hasUtilBar(n.kind)) busiest = Math.max(busiest ?? 0, Math.round(s.util * 100))
  }
  return { count, busiest, down, failing }
}

/** The text a chip shows under the name: "3 components · busiest 87% · 1 down". */
export function chipText(s: ChipSummary): string {
  const parts = [`${s.count} component${s.count === 1 ? '' : 's'}`]
  if (s.busiest !== null) parts.push(`busiest ${s.busiest}%`)
  if (s.down > 0) parts.push(`${s.down} down`)
  if (s.failing > 0) parts.push(`${s.failing} failing`)
  return parts.join(' · ')
}

/** The same diagram with one box collapsed or expanded; nothing else changes. */
export function setCollapsed(d: Diagram, id: string, collapsed: boolean): Diagram {
  const g = groupsOf(d).find((x) => x.id === id)
  if (!g || !!g.collapsed === collapsed) return d
  return {
    ...d,
    groups: groupsOf(d).map((x) => {
      if (x.id !== id) return x
      const { collapsed: _c, ...rest } = x
      void _c
      return collapsed ? { ...rest, collapsed: true } : rest
    }),
  }
}

/** Opens every collapsed box that hides one of these items (used when something hidden is searched for or selected). */
export function expandFor(d: Diagram, ids: string[], rectOf: (n: Node) => Rect): Diagram {
  const want = new Set(ids)
  let out = d
  for (const g of groupsOf(d)) {
    if (!g.collapsed) continue
    if (groupMembers(d, g.id, rectOf).some((m) => want.has(m))) out = setCollapsed(out, g.id, false)
  }
  return out
}
