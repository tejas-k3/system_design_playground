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

  it('tells apart components that share a label by adding their id', () => {
    const twin = (t: number) => dia([node('u', 'client', 'Users'), node('a1', 'appServer', 'App Server', { threads: 8 }), node('a2', 'appServer', 'App Server', { threads: t })], [])
    expect(describeDiff(diffDiagrams(twin(8), twin(16), reg), reg)).toEqual(['App Server (a2): t is 8, the reference uses 16'])
  })
})
