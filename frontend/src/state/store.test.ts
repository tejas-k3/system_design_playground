import { beforeEach, describe, expect, it } from 'vitest'
import type { Scenario, Score } from '../api/types'
import { reg } from '../test/fixtures'
import { useStore } from './store'

const st = () => useStore.getState()

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
})

function setup() {
  const a = st().addNode('appServer', 0, 0)!
  const d = st().addNode('database', 300, 0)!
  return { a, d }
}

const scenario = (configOnly: boolean): Scenario =>
  ({
    id: 's1', title: 'T', difficulty: 'Easy', summary: '', story: '', configOnly, speed: 1, durationSec: 10, warmupSec: 0,
    goals: { minSuccessRate: 0.9 }, hintCount: 3,
    diagram: { nodes: [{ id: 'u', kind: 'client', x: 0, y: 0, label: 'Users', params: {}, locked: true }], edges: [], notes: [] },
  }) as Scenario

describe('diagram editing', () => {
  it('adds uniquely named nodes, selects the new one and supports undo/redo', () => {
    const { a } = setup()
    st().addNode('appServer', 10, 10)
    expect(st().diagram.nodes.map((n) => n.label)).toEqual(['App Server', 'Database', 'App Server 2'])
    expect(st().selection).toHaveLength(1)
    st().undo()
    expect(st().diagram.nodes).toHaveLength(2)
    st().redo()
    expect(st().diagram.nodes).toHaveLength(3)
    expect(st().diagram.nodes[0].id).toBe(a)
  })

  it('groups a drag into a single undo step', () => {
    const { a } = setup()
    const before = st().past.length
    st().beginGesture()
    for (let i = 0; i < 5; i++) st().moveNodes([a], 10, 0)
    st().endGesture()
    expect(st().past.length).toBe(before + 1)
    expect(st().diagram.nodes.find((n) => n.id === a)!.x).toBe(50)
    st().undo()
    expect(st().diagram.nodes.find((n) => n.id === a)!.x).toBe(0)
  })

  it('refuses illegal and duplicate connections', () => {
    const { a, d } = setup()
    expect(st().addEdge(d, a)).toBeNull() // database cannot connect to an app server
    expect(st().toasts.at(-1)?.kind).toBe('warn')
    expect(st().addEdge(a, d)).not.toBeNull()
    expect(st().addEdge(a, d)).toBeNull()
    expect(st().addEdge(a, a)).toBeNull()
    expect(st().diagram.edges).toHaveLength(1)
    expect(st().diagram.edges[0].latencyMs).toBe(5)
  })

  it('deleting a node removes its connections', () => {
    const { a, d } = setup()
    st().addEdge(a, d)
    st().deleteIds([d])
    expect(st().diagram.nodes.map((n) => n.id)).toEqual([a])
    expect(st().diagram.edges).toHaveLength(0)
  })

  it('imports a whole diagram as one undoable step', () => {
    setup()
    const before = st().diagram
    st().importDiagram({ nodes: [{ id: 'x', kind: 'cache', x: 0, y: 0, label: 'Cache', params: {} }], edges: [], notes: [] })
    expect(st().diagram.nodes.map((n) => n.id)).toEqual(['x'])
    expect(st().selection).toEqual([])
    st().undo()
    expect(st().diagram).toEqual(before)
  })

  it('updates parameters without mutating earlier history', () => {
    const { a } = setup()
    st().setParam(a, 'threads', 32)
    expect(st().diagram.nodes[0].params.threads).toBe(32)
    st().undo()
    expect(st().diagram.nodes[0].params.threads).toBeUndefined()
  })
})

describe('challenges', () => {
  it('configuration-only challenges refuse topology changes but allow parameter edits', () => {
    st().openScenario(scenario(true))
    expect(st().addNode('appServer', 0, 0)).toBeNull()
    expect(st().diagram.nodes).toHaveLength(1)
    st().setParam('u', 'rps', 10)
    expect(st().diagram.nodes[0].params.rps).toBe(10)
  })

  it('locked components cannot be deleted', () => {
    st().openScenario(scenario(false))
    st().deleteIds(['u'])
    expect(st().diagram.nodes).toHaveLength(1)
  })

  it('restores the sandbox when leaving a challenge', () => {
    setup()
    st().openScenario(scenario(false))
    expect(st().diagram.nodes.map((n) => n.id)).toEqual(['u'])
    st().exitScenario()
    expect(st().diagram.nodes).toHaveLength(2)
  })

  it('keeps the best star count', () => {
    st().openScenario(scenario(false))
    const score = (stars: number) => ({ pass: stars > 0, stars, checks: [], run: {}, costPerHour: 0 }) as unknown as Score
    st().setResult(score(3))
    st().setResult(score(1))
    expect(st().progress.s1).toBe(3)
  })

  it('reveals at most as many hints as the challenge has, each one arriving with its text', () => {
    st().openScenario(scenario(false))
    expect(st().scenario?.hints).toEqual([])
    for (let i = 0; i < 5; i++) st().addHint(`hint ${i + 1}`)
    expect(st().scenario?.hintLevel).toBe(3)
    expect(st().scenario?.hints).toEqual(['hint 1', 'hint 2', 'hint 3'])
  })

  it('keeps the first answer to a prediction and the debrief once it is fetched', () => {
    st().openScenario(scenario(false))
    const p = { id: 'p1', question: 'q', options: ['a', 'b'] }
    st().recordPrediction(p, { choice: 0, correct: false, answer: 1, explain: 'because' })
    st().recordPrediction(p, { choice: 1, correct: true, answer: 1, explain: 'because' })
    expect(st().scenario?.predictions.p1).toMatchObject({ choice: 0, answer: 1 })
    expect(st().scenario?.debrief).toBeNull()
    st().setDebrief('what to remember')
    expect(st().scenario?.debrief).toBe('what to remember')
  })
})

describe('viewport', () => {
  it('zooming keeps the point under the cursor fixed and clamps the zoom', () => {
    st().setViewport({ scrollX: 40, scrollY: 20, zoom: 1 })
    const world = { x: (200 - 40) / 1, y: (100 - 20) / 1 }
    st().zoomAt(2, 200, 100)
    const v = st().viewport
    expect(v.zoom).toBe(2)
    expect((200 - v.scrollX) / v.zoom).toBeCloseTo(world.x)
    expect((100 - v.scrollY) / v.zoom).toBeCloseTo(world.y)
    st().zoomAt(1000, 0, 0)
    expect(st().viewport.zoom).toBe(4)
    st().zoomAt(1e-6, 0, 0)
    expect(st().viewport.zoom).toBe(0.1)
  })
})

describe('stats', () => {
  it('tolerates a server frame with missing collections', () => {
    st().setStats({ global: { now: 1, inFlight: 0, cost: 0, avgCost: 0, run: st().stats.global.run }, series: null, nodes: null, edges: null } as never)
    expect(st().stats.series).toEqual([])
    expect(st().stats.nodes).toEqual({})
  })

  it('does not know the cost of a diagram until the server has priced it', () => {
    expect(st().cost).toBeNull()
    st().setCost(0.5)
    expect(st().cost).toBe(0.5)
    st().loadDiagram({ nodes: [], edges: [], notes: [] })
    expect(st().cost).toBeNull()
  })

  it('hides the unusable component library in a config-only challenge and brings it back afterwards', () => {
    expect(st().ui.library).toBe(true)
    st().openScenario(scenario(true))
    expect(st().ui.library).toBe(false)
    st().openScenario(scenario(false))
    expect(st().ui.library).toBe(true)
    st().openScenario(scenario(true))
    st().exitScenario()
    expect(st().ui.library).toBe(true)
  })
})

describe('moving an end of an existing connection', () => {
  it('points the same arrow at another component, undoably', () => {
    const { a, d } = setup()
    const d2 = st().addNode('database', 300, 200)!
    const e = st().addEdge(a, d)!
    expect(st().rewireEdge(e, 'to', d2)).toBe(true)
    expect(st().diagram.edges).toHaveLength(1)
    expect(st().diagram.edges[0]).toMatchObject({ id: e, from: a, to: d2 })
    st().undo()
    expect(st().diagram.edges[0]).toMatchObject({ id: e, from: a, to: d })
  })

  it('moves the source end too', () => {
    const { a, d } = setup()
    const a2 = st().addNode('appServer', 0, 200)!
    const e = st().addEdge(a, d)!
    expect(st().rewireEdge(e, 'from', a2)).toBe(true)
    expect(st().diagram.edges[0]).toMatchObject({ id: e, from: a2, to: d })
  })

  it('refuses a connection the kinds do not allow, a duplicate, a loop and a no-op', () => {
    const { a, d } = setup()
    const a2 = st().addNode('appServer', 0, 200)!
    const e = st().addEdge(a, d)!
    st().addEdge(a2, d)
    const before = st().diagram.edges
    expect(st().rewireEdge(e, 'to', a2)).toBe(false) // an app server cannot be fed into an app server here
    expect(st().rewireEdge(e, 'from', a2)).toBe(false) // a2 already sends to d
    expect(st().rewireEdge(e, 'to', a)).toBe(false) // a loop
    expect(st().rewireEdge(e, 'to', d)).toBe(false) // already there
    expect(st().diagram.edges).toEqual(before)
  })
})
