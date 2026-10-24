import type { Diagram, Edge, Group, GroupKind, Node, NodeSpec, Params, ParamSpec, ParamValue, Registry } from '../api/types'
import { GROUP_KIND_IDS, MAX_GROUPS, MAX_GROUP_LABEL, clampGroupSize } from './groups'

export const MIN_ZOOM = 0.1
export const MAX_ZOOM = 4

/** Labels an edge may carry: how the two components talk. They change nothing in the simulation. */
export const PROTOCOLS = ['http', 'grpc', 'tcp', 'ws', 'sql', 'custom']

export const emptyDiagram = (): Diagram => ({ nodes: [], edges: [], notes: [] })

let counter = 0
export function newId(prefix: string): string {
  counter += 1
  return `${prefix}_${Date.now().toString(36)}${counter.toString(36)}${Math.floor(Math.random() * 1296).toString(36)}`
}

export class Reg {
  private byKind = new Map<string, NodeSpec>()
  constructor(public readonly raw: Registry) {
    for (const n of raw.nodes) this.byKind.set(n.kind, n)
  }
  get nodes(): NodeSpec[] {
    return this.raw.nodes
  }
  spec(kind: string): NodeSpec | undefined {
    return this.byKind.get(kind)
  }
  params(kind: string): ParamSpec[] {
    return this.byKind.get(kind)?.params ?? []
  }
  /** Human label of a select setting's value, e.g. 'roundRobin' -> 'Round robin'. */
  optionLabel(kind: string, key: string, value: unknown): string | undefined {
    return this.params(kind).find((p) => p.key === key)?.options?.find((o) => o.value === value)?.label
  }
  defaults(kind: string): Params {
    const out: Params = {}
    for (const p of this.params(kind)) out[p.key] = p.default
    return out
  }
  resolve(node: Pick<Node, 'kind' | 'params'>): Params {
    return { ...this.defaults(node.kind), ...node.params }
  }
  canConnect(from: string, to: string): boolean {
    return !!this.byKind.get(from)?.canConnectTo?.includes(to)
  }
  /** Default one-way latency of a new edge: the first matching server rule wins. */
  defaultLatency(from: string, to: string): number {
    const rule = this.raw.latency.rules.find((r) => (!r.from || r.from === from) && (!r.to || r.to === to))
    return rule?.ms ?? 5
  }
  get speeds(): number[] {
    return this.raw.speeds
  }
  reasonLabel(id: string): string {
    return this.raw.reasons.find((r) => r.id === id)?.label ?? id
  }
}

const isObj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)
const num = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v)
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

export function sanitizeParams(reg: Reg, kind: string, raw: unknown): { params: Params; adjusted: boolean } {
  const params: Params = {}
  if (!isObj(raw)) return { params, adjusted: raw !== undefined && raw !== null }
  let adjusted = false
  const specs = new Map(reg.params(kind).map((p) => [p.key, p]))
  for (const [k, v] of Object.entries(raw)) {
    const ps = specs.get(k)
    if (!ps) {
      adjusted = true
      continue
    }
    let out: ParamValue | undefined
    if (ps.type === 'number') {
      const n = typeof v === 'number' ? v : typeof v === 'string' && v.trim() !== '' ? Number(v) : NaN
      if (Number.isFinite(n)) out = clamp(n, ps.min ?? -Infinity, ps.max ?? Infinity)
      if (out !== n) adjusted = true
    } else if (ps.type === 'select') {
      if (ps.options?.some((o) => o.value === v)) out = v as string
    } else if (typeof v === 'boolean') out = v
    if (out === undefined) adjusted = true
    else params[k] = out
  }
  return { params, adjusted }
}

/** Validate untrusted JSON (localStorage, import) into a safe Diagram. */
export function sanitizeDiagram(reg: Reg, raw: unknown): { diagram: Diagram; adjusted: boolean } | null {
  if (!isObj(raw) || !Array.isArray(raw.nodes)) return null
  let adjusted = false
  const seen = new Set<string>()
  const claim = (id: unknown): id is string => {
    if (typeof id !== 'string' || id === '' || seen.has(id)) return false
    seen.add(id)
    return true
  }
  const nodes: Node[] = []
  for (const n of raw.nodes) {
    if (!isObj(n) || typeof n.kind !== 'string' || !reg.spec(n.kind) || !num(n.x) || !num(n.y) || !claim(n.id)) {
      adjusted = true
      continue
    }
    const sp = sanitizeParams(reg, n.kind, n.params)
    if (sp.adjusted) adjusted = true
    nodes.push({
      id: n.id,
      kind: n.kind,
      x: n.x,
      y: n.y,
      label: typeof n.label === 'string' && n.label ? n.label : reg.spec(n.kind)!.name,
      params: sp.params,
      ...(n.locked === true ? { locked: true } : {}),
    })
  }
  const kinds = new Map(nodes.map((n) => [n.id, n.kind]))
  const pairs = new Set<string>()
  const edges: Edge[] = []
  for (const e of Array.isArray(raw.edges) ? raw.edges : []) {
    if (!isObj(e) || typeof e.from !== 'string' || typeof e.to !== 'string') {
      adjusted = true
      continue
    }
    const fk = kinds.get(e.from)
    const tk = kinds.get(e.to)
    const key = `${e.from}\n${e.to}`
    if (!fk || !tk || e.from === e.to || !reg.canConnect(fk, tk) || pairs.has(key) || !claim(e.id)) {
      adjusted = true
      continue
    }
    pairs.add(key)
    let lat = num(e.latencyMs) && e.latencyMs > 0 ? e.latencyMs : reg.defaultLatency(fk, tk)
    const c = clamp(lat, reg.raw.latency.min, reg.raw.latency.max)
    if (c !== e.latencyMs) adjusted = true
    lat = c
    const out: Edge = { id: e.id as string, from: e.from, to: e.to, latencyMs: lat }
    if (e.bandwidthMbps !== undefined && e.bandwidthMbps !== 0) {
      if (num(e.bandwidthMbps) && e.bandwidthMbps > 0) out.bandwidthMbps = Math.min(100000, e.bandwidthMbps)
      if (out.bandwidthMbps !== e.bandwidthMbps) adjusted = true
    }
    for (const [k, max] of [['lossPct', 100], ['jitterMs', 1000]] as const) {
      const v = e[k]
      if (v === undefined || v === 0) continue
      if (num(v) && v > 0) out[k] = Math.min(max, v)
      if (out[k] !== v) adjusted = true
    }
    if (e.protocol !== undefined) {
      if (typeof e.protocol === 'string' && PROTOCOLS.includes(e.protocol)) out.protocol = e.protocol
      else adjusted = true
    }
    if (e.async !== undefined) {
      if (e.async === true) out.async = true
      else if (e.async !== false) adjusted = true
    }
    edges.push(out)
  }
  const notes = []
  for (const n of Array.isArray(raw.notes) ? raw.notes : []) {
    if (!isObj(n) || !num(n.x) || !num(n.y) || !claim(n.id)) {
      adjusted = true
      continue
    }
    notes.push({ id: n.id as string, x: n.x, y: n.y, text: String(n.text ?? ''), ...(num(n.fontSize) && n.fontSize > 0 ? { fontSize: n.fontSize } : {}) })
  }
  const groups: Group[] = []
  for (const g of Array.isArray(raw.groups) ? raw.groups : []) {
    if (groups.length >= MAX_GROUPS) {
      adjusted = true
      break
    }
    if (!isObj(g) || typeof g.kind !== 'string' || !GROUP_KIND_IDS.includes(g.kind) || !num(g.x) || !num(g.y) || !num(g.w) || !num(g.h) || !claim(g.id)) {
      adjusted = true
      continue
    }
    const w = clampGroupSize(g.w)
    const h = clampGroupSize(g.h)
    let label = typeof g.label === 'string' ? g.label : ''
    if (w !== g.w || h !== g.h || label.length > MAX_GROUP_LABEL || (g.label !== undefined && typeof g.label !== 'string')) adjusted = true
    label = [...label].slice(0, MAX_GROUP_LABEL).join('')
    groups.push({ id: g.id, kind: g.kind as GroupKind, label, x: g.x, y: g.y, w, h, ...(g.collapsed === true ? { collapsed: true } : {}) })
  }
  return { diagram: { nodes, edges, notes, ...(groups.length ? { groups } : {}) }, adjusted }
}

/** Next free label: "Cache", "Cache 2", ... */
export function uniqueLabel(d: Diagram, base: string): string {
  const used = new Set(d.nodes.map((n) => n.label))
  if (!used.has(base)) return base
  for (let i = 2; ; i++) if (!used.has(`${base} ${i}`)) return `${base} ${i}`
}

/** Nodes this node could still be connected to: allowed by the registry, not itself, not already linked. */
export function connectableTargets(d: Diagram, reg: Reg, nodeId: string): Node[] {
  const from = d.nodes.find((n) => n.id === nodeId)
  if (!from) return []
  const linked = new Set(d.edges.filter((e) => e.from === nodeId).map((e) => e.to))
  return d.nodes
    .filter((n) => n.id !== nodeId && !linked.has(n.id) && reg.canConnect(from.kind, n.kind))
    .sort((a, b) => a.label.localeCompare(b.label))
}

export function hasEdgeTo(d: Diagram, nodeId: string, kinds: string[]): boolean {
  const kindOf = new Map(d.nodes.map((n) => [n.id, n.kind]))
  return d.edges.some((e) => e.from === nodeId && kinds.includes(kindOf.get(e.to) ?? ''))
}

export function paramVisible(d: Diagram, node: Node, p: ParamSpec, resolved: Params): boolean {
  if (p.showIf && !p.showIf.in.includes(String(resolved[p.showIf.key]))) return false
  if (p.needsEdgeTo && !hasEdgeTo(d, node.id, p.needsEdgeTo)) return false
  return true
}

/** Components that could send requests somewhere but have no outgoing connection yet (a database sends nothing, so it is never listed).
 *  The canvas shows their connection handle without being asked: it is the next thing to do. */
export function openOutputs(reg: Reg, d: Diagram): Set<string> {
  const sending = new Set(d.edges.map((e) => e.from))
  return new Set(d.nodes.filter((n) => !sending.has(n.id) && (reg.spec(n.kind)?.canConnectTo?.length ?? 0) > 0).map((n) => n.id))
}

/** Why pressing Play would run nothing, or null when there is traffic to simulate. */
export function nothingToRun(d: Diagram): string | null {
  const users = d.nodes.filter((n) => n.kind === 'client')
  if (users.length === 0) return 'Add Users first: they are where the traffic comes from.'
  if (!users.some((u) => d.edges.some((e) => e.from === u.id))) return 'Connect Users to something: they have nowhere to send requests yet.'
  return null
}

/** Give components that share a label distinct ones ("App Server", "App Server 2", ...), as the editor does for what you add, so a reference can be read against your canvas. */
export function uniqueLabels<T extends { nodes: { label: string }[] }>(d: T): T {
  const seen = new Map<string, number>()
  const nodes = d.nodes.map((n) => {
    const k = (seen.get(n.label) ?? 0) + 1
    seen.set(n.label, k)
    return k === 1 ? n : { ...n, label: `${n.label} ${k}` }
  })
  return { ...d, nodes }
}
