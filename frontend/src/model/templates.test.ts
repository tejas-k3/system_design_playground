import { describe, expect, it } from 'vitest'
import { templateDiagram, templates } from './templates'

describe('templates', () => {
  it('has the six starters with unique ids', () => {
    expect(templates.map((t) => t.id)).toEqual(['three-tier', 'cache-aside', 'queue-worker', 'cdn-origin', 'sharded-db', 'primary-replica'])
    expect(new Set(templates.map((t) => t.id)).size).toBe(templates.length)
  })
  it('every connection joins two components in the same template, and ids are unique', () => {
    for (const t of templates) {
      const ids = new Set(t.diagram.nodes.map((n) => n.id))
      expect(ids.size, t.id).toBe(t.diagram.nodes.length)
      for (const e of t.diagram.edges) {
        expect(ids.has(e.from) && ids.has(e.to), `${t.id} ${e.id}`).toBe(true)
        expect(e.from).not.toBe(e.to)
      }
      expect(t.diagram.nodes.some((n) => n.kind === 'client'), `${t.id} needs users`).toBe(true)
    }
  })
  it('hands out copies, so editing one does not change the next', () => {
    const a = templateDiagram('three-tier')!
    a.nodes[0].label = 'changed'
    expect(templateDiagram('three-tier')!.nodes[0].label).toBe('Users')
    expect(templateDiagram('nope')).toBeNull()
  })
})
