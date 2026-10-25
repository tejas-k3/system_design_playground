import { beforeEach, describe, expect, it } from 'vitest'
import type { Scenario } from '../api/types'
import { reg } from '../test/fixtures'
import { useStore } from './store'

const st = () => useStore.getState()
const scenario = { id: 's1', title: 'T', difficulty: 'Easy', summary: '', story: '', speed: 1, durationSec: 10, warmupSec: 0, goals: {}, hintCount: 3, diagram: { nodes: [], edges: [], notes: [] } } as unknown as Scenario

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
})

describe('daily seed', () => {
  it('a challenge opened normally keeps the default seed and shows no daily seed', () => {
    st().openScenario(scenario)
    expect(st().sim.seed).toBe(42)
    expect(st().scenario?.dailySeed).toBeUndefined()
  })
  it('a challenge started from the daily card runs on the given seed and remembers it', () => {
    st().openScenario(scenario, { dailySeed: 5 })
    expect(st().sim.seed).toBe(5)
    expect(st().scenario?.dailySeed).toBe(5)
  })
  it('the daily seed does not leak into the next challenge or the sandbox', () => {
    st().openScenario(scenario, { dailySeed: 5 })
    st().openScenario(scenario)
    expect(st().sim.seed).toBe(42)
    st().openScenario(scenario, { dailySeed: 3 })
    st().exitScenario()
    expect(st().sim.seed).toBe(42)
  })
})
