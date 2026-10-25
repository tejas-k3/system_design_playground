import { beforeEach, describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { reg } from '../test/fixtures'
import { useStore } from './store'

const st = () => useStore.getState()

/** users -> app1, app2 (both inside a zone) -> db (outside) */
function build() {
  const users = st().addNode('client', -400, 100)!
  const app1 = st().addNode('appServer', 40, 80)!
  const app2 = st().addNode('appServer', 40, 200)!
  const db = st().addNode('database', 700, 100)!
  st().addEdge(users, app1)
  st().addEdge(users, app2)
  st().addEdge(app1, db)
  st().addEdge(app2, db)
  const zone = st().addGroup('az', 0, 0)!
  st().updateGroup(zone, { w: 400, h: 400 })
  return { users, app1, app2, db, zone }
}
const flagless = (d: Diagram): Diagram => ({ ...d, groups: d.groups?.map(({ collapsed: _c, ...g }) => (void _c, g)) })
const hiddenFlag = (z: string) => st().diagram.groups!.find((g) => g.id === z)?.collapsed

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
})

describe('collapsing a box in the store', () => {
  it('toggles the flag, selects the box, and undo and redo cover it', () => {
    const { zone } = build()
    st().setSelection([])
    st().toggleCollapsed(zone)
    expect(hiddenFlag(zone)).toBe(true)
    expect(st().selection).toEqual([zone])
    st().toggleCollapsed(zone)
    expect(hiddenFlag(zone)).toBeUndefined()
    st().undo()
    expect(hiddenFlag(zone)).toBe(true)
    st().undo()
    expect(hiddenFlag(zone)).toBeUndefined()
    st().redo()
    expect(hiddenFlag(zone)).toBe(true)
  })

  it('changes nothing but the flag: the design that goes to the simulation is identical', () => {
    const { zone } = build()
    const before = structuredClone(st().diagram)
    st().toggleCollapsed(zone, true)
    expect(JSON.stringify(flagless(st().diagram))).toBe(JSON.stringify(flagless(before)))
    expect(st().diagram.nodes).toBe(st().past[st().past.length - 1].nodes) // not even re-created
    st().toggleCollapsed(zone, false)
    expect(JSON.stringify(st().diagram)).toBe(JSON.stringify(before))
  })

  it('a selection that would be hidden is replaced by the box', () => {
    const { zone, app1 } = build()
    st().setSelection([app1])
    st().toggleCollapsed(zone)
    expect(st().selection).toEqual([zone])
  })

  it('moving a collapsed box takes the hidden members along', () => {
    const { zone, app1, db } = build()
    st().toggleCollapsed(zone, true)
    st().moveNodes([zone], 50, 10)
    expect(st().diagram.nodes.find((n) => n.id === app1)).toMatchObject({ x: 90, y: 90 })
    expect(st().diagram.nodes.find((n) => n.id === db)).toMatchObject({ x: 700, y: 100 })
  })

  it('deleting a collapsed box keeps its components, as for an open box', () => {
    const { zone, app1 } = build()
    st().toggleCollapsed(zone, true)
    st().deleteIds([zone])
    expect(st().diagram.groups ?? []).toHaveLength(0)
    expect(st().diagram.nodes.some((n) => n.id === app1)).toBe(true)
  })

  it('copying a collapsed box copies its members and the copy stays collapsed', () => {
    const { zone } = build()
    st().toggleCollapsed(zone, true)
    st().copySelection(true)
    st().paste()
    const d = st().diagram
    expect(d.nodes).toHaveLength(6) // the two servers inside the box are copied, the two outside are not
    expect(d.groups).toHaveLength(2)
    expect(d.groups![1].collapsed).toBe(true)
    // the pasted selection holds only what is visible: the new box
    expect(st().selection).toEqual([d.groups![1].id])
  })

  it('duplicate (Ctrl+D) of a collapsed box keeps it collapsed', () => {
    const { zone } = build()
    st().toggleCollapsed(zone, true)
    st().duplicateSelection()
    expect(st().diagram.groups!.filter((g) => g.collapsed)).toHaveLength(2)
  })

  it('finding a hidden component expands its box and selects it', () => {
    const { zone, app2 } = build()
    st().toggleCollapsed(zone, true)
    st().focusNode(app2)
    expect(hiddenFlag(zone)).toBeUndefined()
    expect(st().selection).toEqual([app2])
    st().undo()
    expect(hiddenFlag(zone)).toBe(true) // opening it was one undoable step
  })

  it('selecting a hidden component by any route (outline, a lint finding) opens its box', () => {
    const { zone, app1 } = build()
    st().toggleCollapsed(zone, true)
    st().setSelection([app1])
    expect(hiddenFlag(zone)).toBeUndefined()
    expect(st().selection).toEqual([app1])
  })

  it('adding a component inside a collapsed box opens the box, so it does not vanish', () => {
    const { zone } = build()
    st().toggleCollapsed(zone, true)
    st().addNode('appServer', 100, 300)
    expect(hiddenFlag(zone)).toBeUndefined()
  })

  it('tidy up keeps the flag and keeps members inside their box', () => {
    const { zone, app1, app2 } = build()
    st().toggleCollapsed(zone, true)
    st().tidyLayout()
    const d = st().diagram
    const g = d.groups!.find((x) => x.id === zone)!
    expect(g.collapsed).toBe(true)
    for (const id of [app1, app2]) {
      const n = d.nodes.find((x) => x.id === id)!
      expect(n.x).toBeGreaterThanOrEqual(g.x)
      expect(n.y).toBeGreaterThanOrEqual(g.y)
      expect(n.x + 160).toBeLessThanOrEqual(g.x + g.w)
      expect(n.y + 72).toBeLessThanOrEqual(g.y + g.h)
    }
  })
})
