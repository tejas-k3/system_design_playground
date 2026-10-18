import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { clipIsEmpty, copySelection, pasteClip } from './clipboard'

const d: Diagram = {
  nodes: [
    { id: 'a', kind: 'appServer', x: 100, y: 100, label: 'App Server', params: { threads: 8 }, locked: true },
    { id: 'b', kind: 'database', x: 300, y: 100, label: 'Database', params: {} },
    { id: 'c', kind: 'cache', x: 300, y: 300, label: 'Cache', params: {} },
  ],
  edges: [
    { id: 'e1', from: 'a', to: 'b', latencyMs: 2 },
    { id: 'e2', from: 'a', to: 'c', latencyMs: 1, bandwidthMbps: 100 },
  ],
  notes: [{ id: 't1', x: 10, y: 10, text: 'hi' }],
}
const counter = () => {
  let i = 0
  return (p: string) => `${p}${++i}`
}

describe('copy and paste', () => {
  it('copies chosen components and only the connections between them', () => {
    const clip = copySelection(d, ['a', 'b', 't1'])
    expect(clip.nodes.map((n) => n.id)).toEqual(['a', 'b'])
    expect(clip.edges.map((e) => e.id)).toEqual(['e1'])
    expect(clip.notes).toHaveLength(1)
  })

  it('a copy does not change when the diagram does', () => {
    const clip = copySelection(d, ['a'])
    d.nodes[0].params.threads = 99
    expect(clip.nodes[0].params.threads).toBe(8)
    d.nodes[0].params.threads = 8
  })

  it('pastes with fresh ids, free labels, an offset and the internal edges rewired', () => {
    const { diagram, ids } = pasteClip(d, copySelection(d, ['a', 'b']), counter())
    expect(diagram.nodes).toHaveLength(5)
    const [a2, b2] = diagram.nodes.slice(3)
    expect(a2.label).toBe('App Server 2')
    expect(b2.label).toBe('Database 2')
    expect([a2.x, a2.y]).toEqual([124, 124])
    expect(diagram.edges).toHaveLength(3)
    expect(diagram.edges[2]).toMatchObject({ from: a2.id, to: b2.id, latencyMs: 2 })
    expect(ids).toEqual([a2.id, b2.id])
    expect(new Set([...diagram.nodes, ...diagram.edges].map((x) => x.id)).size).toBe(5 + 3)
  })

  it('a pasted copy of a locked component is not locked, and the original is untouched', () => {
    const { diagram } = pasteClip(d, copySelection(d, ['a']), counter())
    expect(diagram.nodes[3].locked).toBeUndefined()
    expect(diagram.nodes[0].locked).toBe(true)
    expect(d.nodes).toHaveLength(3)
  })

  it('pasting twice gives distinct labels', () => {
    const once = pasteClip(d, copySelection(d, ['b']), counter()).diagram
    const twice = pasteClip(once, copySelection(d, ['b']), counter()).diagram
    expect(twice.nodes.map((n) => n.label)).toEqual(['App Server', 'Database', 'Cache', 'Database 2', 'Database 3'])
  })

  it('copying a copy does not stack numbers: Database 2 becomes Database 3, not Database 2 2', () => {
    const once = pasteClip(d, copySelection(d, ['b']), counter()).diagram
    const copyId = once.nodes[3].id
    const twice = pasteClip(once, copySelection(once, [copyId]), counter()).diagram
    expect(twice.nodes[4].label).toBe('Database 3')
  })

  it('knows when there is nothing to paste', () => {
    expect(clipIsEmpty(null)).toBe(true)
    expect(clipIsEmpty(copySelection(d, []))).toBe(true)
    expect(clipIsEmpty(copySelection(d, ['b']))).toBe(false)
  })
})
