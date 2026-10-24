import { describe, expect, it } from 'vitest'
import { placementTag } from './placement'

describe('placementTag', () => {
  it('says nothing for a component the design does not place', () => {
    expect(placementTag({ params: {} })).toBe('')
    expect(placementTag({ params: { threads: 8 } })).toBe('')
  })
  it('names the region, the zone, or both, in words', () => {
    expect(placementTag({ params: { region: 'eu' } })).toBe('eu')
    expect(placementTag({ params: { zone: 'az-b' } })).toBe('az-b')
    expect(placementTag({ params: { region: 'ap', zone: 'az-c' } })).toBe('ap · az-c')
  })
  it('ignores values that are not text', () => {
    expect(placementTag({ params: { region: 3, zone: true } })).toBe('')
  })
})
