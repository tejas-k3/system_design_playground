import type { Diagram, Edge, Node, Note } from '../api/types'
import { uniqueLabel } from './diagram'

/** What copy keeps: the chosen components and notes, and the connections whose both ends are chosen. */
export interface Clip {
  nodes: Node[]
  edges: Edge[]
  notes: Note[]
}

export const PASTE_OFFSET = 24

export function copySelection(d: Diagram, ids: string[]): Clip {
  const pick = new Set(ids)
  const nodes = d.nodes.filter((n) => pick.has(n.id))
  const have = new Set(nodes.map((n) => n.id))
  return {
    nodes: structuredClone(nodes),
    edges: structuredClone(d.edges.filter((e) => have.has(e.from) && have.has(e.to))),
    notes: structuredClone(d.notes.filter((n) => pick.has(n.id))),
  }
}

export const clipIsEmpty = (c: Clip | null): boolean => !c || (c.nodes.length === 0 && c.notes.length === 0)

/**
 * Adds a copy of the clip to the diagram: fresh ids, a free label for each component, everything moved by
 * `offset` so the copy does not sit exactly on the original, and the clip's internal connections kept.
 * Returns the new diagram and the ids of what was added (to select them).
 */
export function pasteClip(d: Diagram, clip: Clip, newId: (prefix: string) => string, offset = PASTE_OFFSET): { diagram: Diagram; ids: string[] } {
  const map = new Map<string, string>()
  let out: Diagram = d
  const nodes: Node[] = []
  for (const n of clip.nodes) {
    const id = newId('n')
    map.set(n.id, id)
    const { locked: _locked, ...rest } = structuredClone(n)
    void _locked
    // the label is chosen against what is already there, including the copies made a moment ago
    const label = uniqueLabel({ ...out, nodes: [...out.nodes, ...nodes] }, n.label.replace(/ \d+$/, ''))
    nodes.push({ ...rest, id, label, x: n.x + offset, y: n.y + offset })
  }
  const edges = clip.edges.filter((e) => map.has(e.from) && map.has(e.to)).map((e) => ({ ...structuredClone(e), id: newId('e'), from: map.get(e.from)!, to: map.get(e.to)! }))
  const notes = clip.notes.map((n) => ({ ...structuredClone(n), id: newId('t'), x: n.x + offset, y: n.y + offset }))
  out = { nodes: [...d.nodes, ...nodes], edges: [...d.edges, ...edges], notes: [...d.notes, ...notes] }
  return { diagram: out, ids: [...nodes.map((n) => n.id), ...notes.map((n) => n.id)] }
}
