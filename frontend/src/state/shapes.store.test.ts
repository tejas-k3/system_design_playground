import { beforeEach, describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import { useStore } from './store'

const st = () => useStore.getState()
const shapes = () => st().diagram.shapes ?? []

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
})

describe('shapes in the store', () => {
  it('adds an ellipse and an arrow, selects the new one, and undoes each', () => {
    const e = st().addShape('ellipse', 10, 20)!
    expect(shapes()[0]).toMatchObject({ id: e, kind: 'ellipse', x: 10, y: 20, label: '' })
    expect(st().selection).toEqual([e])
    const a = st().addShape('arrow', 0, 0)!
    expect(shapes()[1].kind).toBe('arrow')
    expect(st().selection).toEqual([a])
    st().undo()
    st().undo()
    expect(shapes()).toHaveLength(0)
  })

  it('stops at the limit', () => {
    for (let i = 0; i < 42; i++) st().addShape('ellipse', i, i)
    expect(shapes()).toHaveLength(40)
  })

  it('moves with the selection, in one undo step, without carrying the components under it', () => {
    const n = st().addNode('appServer', 50, 50)!
    const s = st().addShape('ellipse', 0, 0)!
    st().moveNodes([s], 30, 40)
    expect(shapes()[0]).toMatchObject({ x: 30, y: 40 })
    expect(st().diagram.nodes.find((x) => x.id === n)).toMatchObject({ x: 50, y: 50 })
    st().undo()
    expect(shapes()[0]).toMatchObject({ x: 0, y: 0 })
  })

  it('resizes an ellipse within limits and sets a caption (truncated)', () => {
    const s = st().addShape('ellipse', 0, 0)!
    st().updateShape(s, { w: 5, h: 99999, label: 'x'.repeat(100) })
    expect(shapes()[0]).toMatchObject({ w: 20, h: 4000 })
    expect(shapes()[0].label).toHaveLength(60)
  })

  it('moving one end of an arrow leaves the other end where it is', () => {
    const s = st().addShape('arrow', 100, 100)! // from (100,100) to (300,100)
    st().moveShapeEnd(s, 'to', 50, 80)
    expect(shapes()[0]).toMatchObject({ x: 100, y: 100, w: 250, h: 80 })
    st().moveShapeEnd(s, 'from', -20, -10)
    // the tip stays at (350,180)
    const a = shapes()[0]
    expect([a.x + a.w, a.y + a.h]).toEqual([350, 180])
    expect([a.x, a.y]).toEqual([80, 90])
  })

  it('deleting removes it and undo brings it back; other things stay', () => {
    const n = st().addNode('appServer', 50, 50)!
    const s = st().addShape('arrow', 0, 0)!
    st().deleteIds([s])
    expect(shapes()).toHaveLength(0)
    expect(st().diagram.nodes.map((x) => x.id)).toEqual([n])
    st().undo()
    expect(shapes()).toHaveLength(1)
  })

  it('copy and paste give a new shape with a fresh id, offset', () => {
    const s = st().addShape('ellipse', 10, 10)!
    st().setSelection([s])
    st().copySelection(true)
    st().paste()
    expect(shapes()).toHaveLength(2)
    expect(shapes()[1].id).not.toBe(s)
    expect(shapes()[1]).toMatchObject({ x: 34, y: 34 })
    expect(st().selection).toEqual([shapes()[1].id])
  })

  it('tidy up layout leaves shapes where they are', () => {
    st().addNode('appServer', 50, 50)
    st().addShape('arrow', 7, 9)
    st().tidyLayout()
    expect(shapes()[0]).toMatchObject({ x: 7, y: 9 })
  })

  it('shapes are not part of what the simulation sees: components and connections are unchanged', () => {
    const a = st().addNode('appServer', 50, 50)!
    const before = JSON.stringify([st().diagram.nodes, st().diagram.edges])
    st().addShape('ellipse', 0, 0)
    expect(JSON.stringify([st().diagram.nodes, st().diagram.edges])).toBe(before)
    expect(a).toBeTruthy()
  })
})
