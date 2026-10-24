import type { Diagram } from '../api/types'
import { Reg, sanitizeDiagram } from './diagram'
import type { Template } from './templates'

export const USER_TEMPLATES_KEY = 'sdp:templates:v1'
export const MAX_USER_TEMPLATES = 20
const MAX_NAME = 40

/** Adds a template (newest first). The design is moved so its top-left corner is the origin, and the list is capped. */
export function addUserTemplate(list: Template[], name: string, design: Diagram, id: string): Template[] {
  const title = name.trim().slice(0, MAX_NAME) || 'My template'
  const copy = structuredClone(design)
  const x0 = Math.min(...copy.nodes.map((n) => n.x), ...copy.notes.map((n) => n.x), ...(copy.groups ?? []).map((n) => n.x), ...(copy.shapes ?? []).map((n) => Math.min(n.x, n.x + n.w)))
  const y0 = Math.min(...copy.nodes.map((n) => n.y), ...copy.notes.map((n) => n.y), ...(copy.groups ?? []).map((n) => n.y), ...(copy.shapes ?? []).map((n) => Math.min(n.y, n.y + n.h)))
  if (Number.isFinite(x0) && Number.isFinite(y0)) {
    for (const n of [...copy.nodes, ...copy.notes, ...(copy.groups ?? []), ...(copy.shapes ?? [])]) {
      n.x -= x0
      n.y -= y0
    }
  }
  const summary = `${copy.nodes.length} components, ${copy.edges.length} connections`
  return [{ id, title, summary, diagram: copy }, ...list].slice(0, MAX_USER_TEMPLATES)
}

export const removeUserTemplate = (list: Template[], id: string): Template[] => list.filter((t) => t.id !== id)

/** Reads the saved list, keeping only entries that are well formed; each design goes through the normal sanitiser. */
export function parseUserTemplates(raw: string | null, reg: Reg): Template[] {
  if (!raw) return []
  let v: unknown
  try {
    v = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(v)) return []
  const out: Template[] = []
  for (const t of v) {
    if (!t || typeof t !== 'object') continue
    const o = t as Record<string, unknown>
    if (typeof o.id !== 'string' || typeof o.title !== 'string') continue
    const s = sanitizeDiagram(reg, o.diagram)
    if (!s || s.diagram.nodes.length === 0) continue
    out.push({ id: o.id, title: o.title.slice(0, MAX_NAME), summary: typeof o.summary === 'string' ? o.summary.slice(0, 120) : '', diagram: s.diagram })
    if (out.length >= MAX_USER_TEMPLATES) break
  }
  return out
}

function storage(): Storage | null {
  try {
    if (typeof window === 'undefined') return null
    window.localStorage.getItem(USER_TEMPLATES_KEY)
    return window.localStorage
  } catch {
    return null
  }
}

export function loadUserTemplates(reg: Reg): Template[] {
  try {
    return parseUserTemplates(storage()?.getItem(USER_TEMPLATES_KEY) ?? null, reg)
  } catch {
    return []
  }
}

export function saveUserTemplates(list: Template[]): void {
  try {
    storage()?.setItem(USER_TEMPLATES_KEY, JSON.stringify(list))
  } catch {
    // storage full or blocked: the list simply is not kept
  }
}
