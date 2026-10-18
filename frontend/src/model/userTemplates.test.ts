import { describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import type { Diagram } from '../api/types'
import { MAX_USER_TEMPLATES, addUserTemplate, parseUserTemplates, removeUserTemplate } from './userTemplates'

const d: Diagram = {
  nodes: [
    { id: 'a', kind: 'appServer', x: 300, y: 200, label: 'App Server', params: { threads: 8 } },
    { id: 'db', kind: 'database', x: 600, y: 200, label: 'Database', params: {} },
  ],
  edges: [{ id: 'e', from: 'a', to: 'db', latencyMs: 5 }],
  notes: [],
}

describe('user templates', () => {
  it('adds a template with a trimmed name and the design moved to the origin', () => {
    const list = addUserTemplate([], '  My pair  ', d, 'u1')
    expect(list).toHaveLength(1)
    expect(list[0]).toMatchObject({ id: 'u1', title: 'My pair' })
    expect(list[0].diagram.nodes.map((n) => [n.x, n.y])).toEqual([[0, 0], [300, 0]])
    expect(d.nodes[0].x).toBe(300) // the original is untouched
  })
  it('falls back to a default name and caps the length', () => {
    expect(addUserTemplate([], '   ', d, 'u')[0].title).toBe('My template')
    expect(addUserTemplate([], 'x'.repeat(200), d, 'u')[0].title).toHaveLength(40)
  })
  it('keeps at most the newest MAX_USER_TEMPLATES', () => {
    let list = [] as ReturnType<typeof addUserTemplate>
    for (let i = 0; i < MAX_USER_TEMPLATES + 3; i++) list = addUserTemplate(list, `t${i}`, d, `id${i}`)
    expect(list).toHaveLength(MAX_USER_TEMPLATES)
    expect(list[0].title).toBe(`t${MAX_USER_TEMPLATES + 2}`)
  })
  it('removes by id', () => {
    const list = addUserTemplate(addUserTemplate([], 'a', d, '1'), 'b', d, '2')
    expect(removeUserTemplate(list, '1').map((t) => t.id)).toEqual(['2'])
  })
  it('reads back what it saved, and survives garbage', () => {
    const list = addUserTemplate([], 'pair', d, 'u1')
    expect(parseUserTemplates(JSON.stringify(list), reg).map((t) => t.id)).toEqual(['u1'])
    expect(parseUserTemplates(null, reg)).toEqual([])
    expect(parseUserTemplates('{not json', reg)).toEqual([])
    expect(parseUserTemplates('{"a":1}', reg)).toEqual([])
    expect(parseUserTemplates(JSON.stringify([{ id: 1 }, { id: 'x', title: 't', diagram: 'no' }]), reg)).toEqual([])
  })
  it('sanitises a stored design (unknown kinds are dropped)', () => {
    const bad = [{ id: 'x', title: 't', summary: '', diagram: { nodes: [{ id: 'z', kind: 'nope', x: 0, y: 0, label: 'Z', params: {} }, d.nodes[1]], edges: [], notes: [] } }]
    const out = parseUserTemplates(JSON.stringify(bad), reg)
    expect(out[0].diagram.nodes.map((n) => n.id)).toEqual(['db'])
  })
})
