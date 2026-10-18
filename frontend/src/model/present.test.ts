import { describe, expect, it } from 'vitest'
import { defaultEstimate, emptyInterview } from './interview'
import { presentSlide } from './present'

const data = {
  ...emptyInterview(),
  functional: ['Upload a photo', ''],
  nonFunctional: [{ label: 'Availability', target: 99.9, unit: '%' }, { label: 'p95', target: null, unit: '' }],
  estimate: { ...defaultEstimate(), mau: 300e6, dauPct: 50, actionsPerDay: 2, perServerQps: 400, headroomPct: 30 },
  api: [{ method: 'POST', path: '/photos', request: '{file}', response: '201', notes: '' }],
  tables: [{ name: 'photos', fields: 'id', key: 'id', indexes: '', access: 'by owner' }],
  rubric: { scalability: 4, availability: 0, consistency: 3, cost: 0, communication: 5 },
  notes: 'Chose a cache.',
}

describe('presentSlide', () => {
  it('lists the requirements, skipping blank lines and marking missing numbers', () => {
    const s = presentSlide('requirements', data)
    expect(s.kind).toBe('lists')
    expect(s.kind === 'lists' && s.groups).toEqual([
      { title: 'Functional', items: ['Upload a photo'] },
      { title: 'Non-functional', items: ['Availability: 99.9 %', 'p95: no number yet'] },
    ])
  })
  it('shows the estimate as label and value rows', () => {
    const s = presentSlide('estimation', data)
    expect(s.kind === 'facts' && s.rows.find((r) => r[0] === 'Peak load')?.[1]).toBe('10,417 /s')
    expect(s.kind === 'facts' && s.rows.find((r) => r[0] === 'App servers needed')?.[1]).toBe('38')
  })
  it('shows API and data pads as tables', () => {
    const a = presentSlide('api', data)
    expect(a.kind === 'table' && a.head).toEqual(['Method', 'Path', 'In', 'Out', 'Notes'])
    expect(a.kind === 'table' && a.rows[0]).toEqual(['POST', '/photos', '{file}', '201', ''])
    const d = presentSlide('data', data)
    expect(d.kind === 'table' && d.rows[0][0]).toBe('photos')
  })
  it('leaves design and deep dive to the canvas', () => {
    expect(presentSlide('design', data).kind).toBe('canvas')
    expect(presentSlide('deepdive', data).kind).toBe('canvas')
  })
  it('wrap-up shows ratings in words and the notes', () => {
    const s = presentSlide('tradeoffs', data)
    expect(s.kind === 'lists' && s.groups[0]).toEqual({ title: 'Self-assessment', items: ['Scalability: 4 / 5', 'Availability: not rated', 'Consistency: 3 / 5', 'Cost: not rated', 'Communication: 5 / 5'] })
    expect(s.kind === 'lists' && s.groups[1]).toEqual({ title: 'Notes', items: ['Chose a cache.'] })
  })
})
