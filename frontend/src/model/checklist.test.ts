import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { checklistProgress, type StarterStep } from './checklist'

const steps: StarterStep[] = [
  { need: 'Something that sends the requests', kind: 'client', nudge: 'n1' },
  { need: 'Something that runs your code', kind: 'appServer', nudge: 'n2' },
  { need: 'Somewhere to keep the links', kind: 'database', nudge: 'n3' },
]
const node = (id: string, kind: string) => ({ id, kind, x: 0, y: 0, label: id, params: {} })
const edge = (from: string, to: string) => ({ id: `${from}>${to}`, from, to, latencyMs: 1 })
const design = (nodes: [string, string][], edges: [string, string][]): Diagram => ({ nodes: nodes.map(([i, k]) => node(i, k)), edges: edges.map(([a, b]) => edge(a, b)), notes: [] })

describe('checklistProgress', () => {
  it('ticks only what is already on the canvas and reachable from the users', () => {
    expect(checklistProgress(design([['users', 'client']], []), steps).map((s) => s.done)).toEqual([true, false, false])
  })
  it('does not tick a component the users cannot reach', () => {
    const d = design([['users', 'client'], ['app', 'appServer'], ['db', 'database']], [['app', 'db']])
    expect(checklistProgress(d, steps).map((s) => s.done)).toEqual([true, false, false])
  })
  it('follows the connections through a balancer', () => {
    const d = design([['users', 'client'], ['lb', 'loadBalancer'], ['app', 'appServer'], ['db', 'database']], [['users', 'lb'], ['lb', 'app'], ['app', 'db']])
    expect(checklistProgress(d, steps).every((s) => s.done)).toBe(true)
  })
  it('keeps the order and the text of the steps', () => {
    const out = checklistProgress(design([], []), steps)
    expect(out.map((s) => s.need)).toEqual(steps.map((s) => s.need))
    expect(out.every((s) => !s.done)).toBe(true)
  })
  it('copes with a design that has no users at all', () => {
    expect(checklistProgress(design([['app', 'appServer']], []), steps).map((s) => s.done)).toEqual([false, false, false])
  })
  it('needs each step to be reached from the one above it, not just to exist', () => {
    const direct = design([['users', 'client'], ['db', 'database']], [['users', 'db']])
    expect(checklistProgress(direct, steps).map((s) => s.done)).toEqual([true, false, false])
  })
  it('does not tick the last step when it hangs off a different path than the app server', () => {
    const d = design([['users', 'client'], ['app', 'appServer'], ['cache', 'cache'], ['db', 'database']], [['users', 'app'], ['users', 'db']])
    expect(checklistProgress(d, steps).map((s) => s.done)).toEqual([true, true, false])
  })
  it('ticks the last step once the app server feeds it, even through another component', () => {
    const d = design([['users', 'client'], ['app', 'appServer'], ['cache', 'cache'], ['db', 'database']], [['users', 'app'], ['app', 'cache'], ['cache', 'db']])
    expect(checklistProgress(d, steps).every((s) => s.done)).toBe(true)
  })
  it('ignores an edge that points back at the users', () => {
    const d = design([['users', 'client'], ['app', 'appServer']], [['app', 'users']])
    expect(checklistProgress(d, steps).map((s) => s.done)).toEqual([true, false, false])
  })
})
