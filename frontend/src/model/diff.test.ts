import { describe, expect, it } from 'vitest'
import type { Diagram, Node } from '../api/types'
import { reg } from '../test/fixtures'
import { describeDiff, diffDiagrams } from './diff'

const node = (id: string, kind: string, label: string, params = {}): Node => ({ id, kind, x: 0, y: 0, label, params })
const edge = (from: string, to: string) => ({ id: `${from}>${to}`, from, to, latencyMs: 5 })
const dia = (nodes: Node[], edges = [] as Diagram['edges']): Diagram => ({ nodes, edges, notes: [] }) as unknown as Diagram

const mine = dia([node('u', 'client', 'Users'), node('a1', 'appServer', 'App 1', { threads: 8 }), node('db', 'database', 'DB')], [edge('u', 'a1'), edge('a1', 'db')])

describe('diffDiagrams', () => {
  it('finds nothing between identical designs, whatever the order and whether defaults are spelled out', () => {
    const same = dia([node('db', 'database', 'DB'), node('a1', 'appServer', 'App 1', { threads: 8, order: 'fifo' }), node('u', 'client', 'Users')], [edge('a1', 'db'), edge('u', 'a1')])
    const d = diffDiagrams(mine, same, reg)
    expect(d).toMatchObject({ addedNodes: [], removedNodes: [], addedEdges: [], removedEdges: [], changed: [] })
    expect(describeDiff(d, reg)).toEqual([])
  })

  it('lists components and connections the reference has and you lack, and the reverse', () => {
    const ref = dia([...mine.nodes, node('a2', 'appServer', 'App 2')], [...mine.edges, edge('u', 'a2'), edge('a2', 'db')])
    const d = diffDiagrams(mine, ref, reg)
    expect(d.addedNodes.map((n) => n.label)).toEqual(['App 2'])
    expect(d.addedEdges.map((e) => e.id)).toEqual(['u>a2', 'a2>db'])
    const back = diffDiagrams(ref, mine, reg)
    expect(back.removedNodes.map((n) => n.label)).toEqual(['App 2'])
    expect(back.removedEdges).toHaveLength(2)
  })

  it('reports each changed setting with its human label and both values', () => {
    const ref = dia([node('u', 'client', 'Users'), node('a1', 'appServer', 'App 1', { threads: 16, order: 'lifo', cb: true }), node('db', 'database', 'DB')], mine.edges)
    const d = diffDiagrams(mine, ref, reg)
    expect(d.changed).toEqual([
      { nodeId: 'a1', node: 'App 1', setting: 't', mine: '8', reference: '16' },
      { nodeId: 'a1', node: 'App 1', setting: 'o', mine: 'Oldest first', reference: 'Newest first' },
      { nodeId: 'a1', node: 'App 1', setting: 'c', mine: 'off', reference: 'on' },
    ])
  })

  it('describes every difference in words', () => {
    const ref = dia([node('u', 'client', 'Users'), node('a1', 'appServer', 'App 1', { threads: 16 }), node('a2', 'appServer', 'App 2'), node('db', 'database', 'DB')], [...mine.edges, edge('a2', 'db')])
    expect(describeDiff(diffDiagrams(mine, ref, reg), reg)).toEqual([
      'Add App 2 (App Server)',
      'Connect App 2 to DB',
      'App 1: t is 8, the reference uses 16',
    ])
  })

  it('says what to remove when you have something the reference does not', () => {
    const extra = dia([...mine.nodes, node('c', 'database', 'Spare')], [...mine.edges, edge('a1', 'c')])
    expect(describeDiff(diffDiagrams(extra, mine, reg), reg)).toEqual(['Remove Spare (Database)', 'Remove the connection from App 1 to Spare'])
  })

  it('pairs components by what they connect to, not by their ids, so a hand-built design is not "different"', () => {
    const ref = dia(
      [node('u', 'client', 'Users'), node('lb', 'loadBalancer', 'Load Balancer'), node('app1', 'appServer', 'App Server', { threads: 8 }), node('app2', 'appServer', 'App Server', { threads: 8 }), node('cache', 'cache', 'Cache'), node('db', 'database', 'Database')],
      [edge('u', 'lb'), edge('lb', 'app1'), edge('lb', 'app2'), edge('app1', 'cache'), edge('app2', 'cache'), edge('cache', 'db')],
    )
    const built = dia(
      [node('n_x1', 'client', 'Users'), node('n_x2', 'loadBalancer', 'Load Balancer'), node('n_x3', 'appServer', 'App Server', { threads: 8 }), node('n_x4', 'appServer', 'App Server 2', { threads: 8 }), node('n_x5', 'cache', 'Cache'), node('n_x6', 'database', 'Database')],
      [edge('n_x1', 'n_x2'), edge('n_x2', 'n_x3'), edge('n_x2', 'n_x4'), edge('n_x3', 'n_x5'), edge('n_x4', 'n_x5'), edge('n_x5', 'n_x6')],
    )
    const d = diffDiagrams(built, ref, reg)
    expect(describeDiff(d, reg)).toEqual([])
    expect(d.sameArchitecture).toBe(true)
  })

  it('names components as they are on your canvas and never shows an internal id', () => {
    const ref = dia([node('u', 'client', 'Users'), node('app1', 'appServer', 'App Server', { threads: 16 }), node('app2', 'appServer', 'App Server', { threads: 8 })], [edge('u', 'app1'), edge('u', 'app2')])
    const built = dia([node('n_a', 'client', 'Users'), node('n_b', 'appServer', 'App Server', { threads: 8 }), node('n_c', 'appServer', 'App Server 2', { threads: 8 })], [edge('n_a', 'n_b'), edge('n_a', 'n_c')])
    const lines = describeDiff(diffDiagrams(built, ref, reg), reg)
    expect(lines).toEqual(['App Server: t is 8, the reference uses 16'])
    expect(lines.join(' ')).not.toMatch(/n_[abc]|app1|app2/)
  })

  it('numbers a reference-only component after your own labels so a sentence cannot be ambiguous', () => {
    const ref = dia([node('u', 'client', 'Users'), node('c1', 'cache', 'Cache'), node('a1', 'appServer', 'App Server')], [edge('u', 'a1'), edge('a1', 'c1')])
    const built = dia([node('u', 'client', 'Users'), node('a1', 'appServer', 'App Server'), node('x', 'cache', 'Cache')], [edge('u', 'a1')])
    // the cache pairs with yours by kind; the connection to it is missing
    expect(describeDiff(diffDiagrams(built, ref, reg), reg)).toEqual(['Connect App Server to Cache'])
  })
})

describe('uniqueLabels', () => {
  it('numbers components that share a label, in order, and leaves the rest alone', async () => {
    const { uniqueLabels } = await import('./diagram')
    const d = uniqueLabels(dia([node('a', 'appServer', 'App Server'), node('b', 'appServer', 'App Server'), node('c', 'database', 'DB'), node('d', 'appServer', 'App Server')]))
    expect(d.nodes.map((n) => n.label)).toEqual(['App Server', 'App Server 2', 'DB', 'App Server 3'])
  })
})
