import type { Diagram, Edge, Node, NodeSpec, Params, ParamSpec, ParamValue, Registry } from '../api/types'

export const MIN_ZOOM = 0.1
export const MAX_ZOOM = 4

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
    edges.push({ id: e.id as string, from: e.from, to: e.to, latencyMs: lat })
  }
  const notes = []
  for (const n of Array.isArray(raw.notes) ? raw.notes : []) {
    if (!isObj(n) || !num(n.x) || !num(n.y) || !claim(n.id)) {
      adjusted = true
      continue
    }
    notes.push({ id: n.id as string, x: n.x, y: n.y, text: String(n.text ?? ''), ...(num(n.fontSize) && n.fontSize > 0 ? { fontSize: n.fontSize } : {}) })
  }
  return { diagram: { nodes, edges, notes }, adjusted }
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
