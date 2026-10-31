import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { guaranteeNotJudged } from './judged'

const node = (id: string, kind: string, params: Record<string, string | number | boolean> = {}) => ({ id, kind, x: 0, y: 0, label: id, params })
const edge = (from: string, to: string) => ({ id: `${from}>${to}`, from, to, latencyMs: 1 })
const design = (model: string, store: string, extra: { id: string; kind: string }[] = []): Diagram => ({
  nodes: [node('users', 'client', { consistency: model }), node('app', 'appServer'), node('store', store), ...extra.map((n) => node(n.id, n.kind))],
  edges: [edge('users', 'app'), edge('app', 'store')],
  notes: [],
})

describe('guaranteeNotJudged', () => {
  it('is true when writes-follow-reads is asked of a plain database', () => {
    expect(guaranteeNotJudged(design('writesFollowReads', 'database'))).toBe(true)
  })
  it('is false against a key-value store', () => {
    expect(guaranteeNotJudged(design('writesFollowReads', 'kvStore'))).toBe(false)
  })
  it('is false when any store reachable from the users is a key-value store', () => {
    const d = design('writesFollowReads', 'database', [{ id: 'kv', kind: 'kvStore' }])
    d.edges.push(edge('app', 'kv'))
    expect(guaranteeNotJudged(d)).toBe(false)
  })
  it('is false for guarantees every store can judge, and for no guarantee', () => {
    expect(guaranteeNotJudged(design('monotonicWrites', 'database'))).toBe(false)
    expect(guaranteeNotJudged(design('readYourWrites', 'database'))).toBe(false)
    expect(guaranteeNotJudged(design('eventual', 'database'))).toBe(false)
  })
  it('says nothing when the users reach no store at all', () => {
    const d = design('writesFollowReads', 'database')
    d.edges = [edge('users', 'app')]
    expect(guaranteeNotJudged(d)).toBe(false)
  })
  it('ignores a store the users cannot reach', () => {
    const d = design('writesFollowReads', 'database', [{ id: 'island', kind: 'kvStore' }])
    expect(guaranteeNotJudged(d)).toBe(true)
  })
})
