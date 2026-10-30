import { describe, expect, it } from 'vitest'
import type { BottleneckPoint, Diagram } from '../api/types'
import { componentOptions, rowsWithFirst } from './bottleneck'

const node = (id: string, kind: string, label: string) => ({ id, kind, x: 0, y: 0, label, params: {} })
const diagram = (nodes: ReturnType<typeof node>[]): Diagram => ({ nodes, edges: [], notes: [] })

describe('componentOptions', () => {
  it('lists every part except the users, in the order drawn', () => {
    const d = diagram([node('users', 'client', 'Users'), node('app', 'appServer', 'App Server'), node('db', 'database', 'Database')])
    expect(componentOptions(d)).toEqual([{ id: 'app', text: 'App Server' }, { id: 'db', text: 'Database' }])
  })
  it('tells apart parts that share a name', () => {
    const d = diagram([node('app1', 'appServer', 'App Server'), node('app2', 'appServer', 'App Server'), node('db', 'database', 'Database')])
    expect(componentOptions(d).map((o) => o.text)).toEqual(['App Server (app1)', 'App Server (app2)', 'Database'])
  })
  it('is empty for a design with only users', () => {
    expect(componentOptions(diagram([node('users', 'client', 'Users')]))).toEqual([])
  })
})

describe('rowsWithFirst', () => {
  const pts: BottleneckPoint[] = [1, 1.5, 2].map((load) => ({ load, rps: load * 100, successRate: 1, p95: 100, hottest: 'db', hottestUtil: 0.5 }))
  it('marks the load where something first runs out', () => {
    expect(rowsWithFirst(pts, 1).map((r) => r.first)).toEqual([false, true, false])
  })
  it('marks nothing when nothing runs out', () => {
    expect(rowsWithFirst(pts, -1).every((r) => !r.first)).toBe(true)
  })
})
