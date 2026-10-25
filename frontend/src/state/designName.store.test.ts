import { beforeEach, describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import { useStore } from './store'

const st = () => useStore.getState()

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
})

describe('renaming the design', () => {
  it('sets a normalised name as one undo step', () => {
    st().setDiagramName('  Shop\u0000  ')
    expect(st().diagram.name).toBe('Shop')
    st().undo()
    expect(st().diagram.name).toBeUndefined()
    st().redo()
    expect(st().diagram.name).toBe('Shop')
  })
  it('an empty name removes the field so the default shows; the same name is not a new step', () => {
    st().setDiagramName('Shop')
    const steps = st().past.length
    st().setDiagramName('Shop')
    expect(st().past.length).toBe(steps)
    st().setDiagramName('   ')
    expect('name' in st().diagram).toBe(false)
  })
  it('is cut to 60 characters', () => {
    st().setDiagramName('x'.repeat(200))
    expect(st().diagram.name).toHaveLength(60)
  })
  it('survives editing the canvas and applying a template, and a new design starts unnamed', () => {
    st().setDiagramName('Shop')
    st().addNode('appServer', 0, 0)
    expect(st().diagram.name).toBe('Shop')
    st().applyTemplate({ nodes: [], edges: [], notes: [] })
    expect(st().diagram.name).toBe('Shop')
    st().newDiagram()
    expect(st().diagram.name).toBeUndefined()
  })
})
