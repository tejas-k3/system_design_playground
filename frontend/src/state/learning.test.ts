import { beforeEach, describe, expect, it } from 'vitest'
import type { Scenario } from '../api/types'
import { loadAnswer, loadDebrief } from './learning'
import { reg } from '../test/fixtures'
import { useStore } from './store'

const st = () => useStore.getState()
const def = { id: 's1', title: 'T', difficulty: 'Hard', summary: '', story: '', speed: 1, durationSec: 10, warmupSec: 0, goals: {}, hintCount: 3, diagram: { nodes: [], edges: [], notes: [] } } as unknown as Scenario

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
  st().openScenario(def)
})

describe('Go deeper', () => {
  it('stores the deeper text when the debrief response has it', async () => {
    await loadDebrief('s1', async () => ({ debrief: 'short', deeper: 'long form' }))
    expect(st().scenario).toMatchObject({ debrief: 'short', deeper: 'long form' })
  })
  it('has no deeper text when the response lacks it, or it is empty', async () => {
    await loadDebrief('s1', async () => ({ debrief: 'short' }))
    expect(st().scenario).toMatchObject({ debrief: 'short', deeper: null })
    await loadDebrief('s1', async () => ({ debrief: 'short', deeper: '' }))
    expect(st().scenario!.deeper).toBeNull()
  })
  it('leaves the debrief unset when the request fails', async () => {
    await loadDebrief('s1', async () => { throw new Error('boom') })
    expect(st().scenario!.debrief).toBeNull()
  })
})

describe('Show the answer', () => {
  it('stores the answer and records that it was used', async () => {
    expect(await loadAnswer('s1', async () => ({ hint: 'Run 5' }))).toBe(true)
    expect(st().scenario!.answer).toBe('Run 5')
    expect(st().records.s1.answerUsed).toBe(true)
  })
  it('on a 404 or empty reply records nothing and tells the learner', async () => {
    expect(await loadAnswer('s1', async () => { throw new Error('Not Found') })).toBe(false)
    expect(await loadAnswer('s1', async () => ({ hint: undefined }))).toBe(false)
    expect(st().scenario!.answer).toBeNull()
    expect(st().records.s1).toBeUndefined()
    expect(st().toasts.length).toBeGreaterThan(0)
  })
})
