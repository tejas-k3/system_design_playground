import type { Diagram, Group, GroupKind } from '../api/types'
import type { Rect } from '../draw/geometry'

/**
 * Groups are labelled boxes behind the components (an availability zone, a region, a VPC, a service boundary, a plain
 * box). They are visual only: the simulation ignores them. What is "inside" is decided by geometry, never stored.
 * Each kind has its own text tag and line pattern, so the meaning never rests on colour alone.
 */
export interface GroupKindInfo {
  kind: GroupKind
  name: string
  tag: string // short text shown in the box header
  dash: number[] // line pattern: [] is solid
}

export const GROUP_KINDS: GroupKindInfo[] = [
  { kind: 'region', name: 'Region', tag: 'Region', dash: [14, 6] },
  { kind: 'az', name: 'Availability zone', tag: 'AZ', dash: [8, 6] },
  { kind: 'vpc', name: 'VPC / network', tag: 'VPC', dash: [2, 5] },
  { kind: 'service', name: 'Service', tag: 'Service', dash: [] },
  { kind: 'note', name: 'Box', tag: 'Box', dash: [3, 3] },
]
export const GROUP_KIND_IDS: string[] = GROUP_KINDS.map((k) => k.kind)
export const MAX_GROUPS = 30
export const MIN_GROUP = 60
export const MAX_GROUP = 4000
export const MAX_GROUP_LABEL = 60
export const GROUP_HEADER = 30
export const GROUP_DEFAULT = { w: 320, h: 220 }

export const groupInfo = (kind: string): GroupKindInfo => GROUP_KINDS.find((k) => k.kind === kind) ?? GROUP_KINDS[4]
export const groupRect = (g: Group): Rect => ({ x: g.x, y: g.y, w: g.w, h: g.h })
export const groupsOf = (d: Diagram): Group[] => d.groups ?? []
/** The text a screen reader or a header shows: "AZ: us-east-1a". */
export const groupTitle = (g: Group): string => (g.label ? `${groupInfo(g.kind).tag}: ${g.label}` : groupInfo(g.kind).name)

const contains = (outer: Rect, inner: Rect): boolean => inner.x >= outer.x && inner.y >= outer.y && inner.x + inner.w <= outer.x + outer.w && inner.y + inner.h <= outer.y + outer.h
const centreIn = (outer: Rect, r: Rect): boolean => {
  const cx = r.x + r.w / 2
  const cy = r.y + r.h / 2
  return cx >= outer.x && cx <= outer.x + outer.w && cy >= outer.y && cy <= outer.y + outer.h
}

type NodeRect = (n: Diagram['nodes'][number]) => Rect

/** What sits inside a group: components whose centre is in the box, notes whose corner is, and smaller groups wholly in it. */
export function groupMembers(d: Diagram, groupId: string, nodeRect: NodeRect): string[] {
  const g = groupsOf(d).find((x) => x.id === groupId)
  if (!g) return []
  const box = groupRect(g)
  const ids: string[] = []
  for (const n of d.nodes) if (centreIn(box, nodeRect(n))) ids.push(n.id)
  for (const n of d.notes) if (n.x >= box.x && n.x <= box.x + box.w && n.y >= box.y && n.y <= box.y + box.h) ids.push(n.id)
  for (const o of groupsOf(d)) if (o.id !== g.id && o.w * o.h < g.w * g.h && contains(box, groupRect(o))) ids.push(o.id)
  return ids
}

/** The chosen ids plus everything inside any chosen group, so moving or copying a box takes its contents along. */
export function withMembers(d: Diagram, ids: string[], nodeRect: NodeRect): string[] {
  const out = new Set(ids)
  for (const id of ids) for (const m of groupMembers(d, id, nodeRect)) out.add(m)
  return [...out]
}

/** A free label for a new group of this kind: "Availability zone", "Availability zone 2", ... */
export function groupLabel(d: Diagram, kind: GroupKind): string {
  const base = groupInfo(kind).name
  const used = new Set(groupsOf(d).map((g) => g.label))
  if (!used.has(base)) return base
  for (let i = 2; ; i++) if (!used.has(`${base} ${i}`)) return `${base} ${i}`
}

export const clampGroupSize = (v: number): number => Math.min(MAX_GROUP, Math.max(MIN_GROUP, v))
