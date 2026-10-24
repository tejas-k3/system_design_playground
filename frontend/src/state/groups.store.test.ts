import { beforeEach, describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import { useStore } from './store'

const st = () => useStore.getState()

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
})

describe('groups in the store', () => {
  it('adds a labelled group, selects it, and undoes', () => {
    const id = st().addGroup('az', 10, 20)!
    expect(st().diagram.groups).toHaveLength(1)
    expect(st().diagram.groups![0]).toMatchObject({ id, kind: 'az', label: 'Availability zone', x: 10, y: 20 })
    expect(st().selection).toEqual([id])
    st().addGroup('az', 0, 0)
    expect(st().diagram.groups![1].label).toBe('Availability zone 2')
    st().undo()
    st().undo()
    expect(st().diagram.groups ?? []).toHaveLength(0)
  })

  it('moving a group moves what is inside it, in one undo step per move, and leaves the outside alone', () => {
    const inside = st().addNode('appServer', 50, 50)!
    const outside = st().addNode('database', 900, 50)!
    const note = st().addNote(60, 150)
    const g = st().addGroup('region', 0, 0)!
    st().moveNodes([g], 30, 40)
    const n = (id: string) => st().diagram.nodes.find((x) => x.id === id)!
    expect([n(inside).x, n(inside).y]).toEqual([80, 90])
    expect([n(outside).x, n(outside).y]).toEqual([900, 50])
    expect(st().diagram.notes.find((x) => x.id === note)).toMatchObject({ x: 90, y: 190 })
    expect(st().diagram.groups![0]).toMatchObject({ x: 30, y: 40 })
    st().undo()
    expect(n(inside).x).toBe(50)
    expect(st().diagram.groups![0].x).toBe(0)
  })

  it('moving a group and one of its members together moves the member once', () => {
    const inside = st().addNode('appServer', 50, 50)!
    const g = st().addGroup('az', 0, 0)!
    st().moveNodes([g, inside], 10, 0)
    expect(st().diagram.nodes[0].x).toBe(60)
  })

  it('moving a member leaves the group where it is', () => {
    const inside = st().addNode('appServer', 50, 50)!
    st().addGroup('az', 0, 0)
    st().moveNodes([inside], 10, 0)
    expect(st().diagram.groups![0].x).toBe(0)
  })

  it('deleting a group keeps its contents; deleting a component keeps the groups', () => {
    const a = st().addNode('appServer', 50, 50)!
    const g = st().addGroup('az', 0, 0)!
    st().deleteIds([a])
    expect(st().diagram.groups).toHaveLength(1)
    const a2 = st().addNode('appServer', 50, 50)!
    st().deleteIds([g])
    expect(st().diagram.groups).toHaveLength(0)
    expect(st().diagram.nodes.map((n) => n.id)).toEqual([a2])
  })

  it('edits keep the groups', () => {
    st().addGroup('vpc', 0, 0)
    const a = st().addNode('appServer', 50, 50)!
    const b = st().addNode('database', 400, 50)!
    st().addEdge(a, b)
    st().deleteIds([b])
    expect(st().diagram.groups).toHaveLength(1)
  })

  it('renames, retypes and resizes within limits', () => {
    const g = st().addGroup('az', 0, 0)!
    st().updateGroup(g, { label: 'us-east-1a', kind: 'region', w: 10, h: 99999 })
    expect(st().diagram.groups![0]).toMatchObject({ label: 'us-east-1a', kind: 'region', w: 60, h: 4000 })
    st().updateGroup(g, { label: 'x'.repeat(100) })
    expect(st().diagram.groups![0].label).toHaveLength(60)
  })

  it('stops at the group limit', () => {
    for (let i = 0; i < 32; i++) st().addGroup('note', i, i)
    expect(st().diagram.groups).toHaveLength(30)
  })

  it('copy and paste take a group with its contents, with fresh ids', () => {
    const a = st().addNode('appServer', 50, 50)!
    const g = st().addGroup('az', 0, 0)!
    st().setSelection([g])
    st().copySelection(true)
    st().paste()
    const d = st().diagram
    expect(d.groups).toHaveLength(2)
    expect(d.nodes).toHaveLength(2)
    expect(d.groups![1].id).not.toBe(g)
    expect(d.nodes[1].id).not.toBe(a)
    expect(d.groups![1]).toMatchObject({ x: 24, y: 24 })
    expect(d.nodes[1]).toMatchObject({ x: 74, y: 74 })
    expect(st().selection).toContain(d.groups![1].id)
  })
})
