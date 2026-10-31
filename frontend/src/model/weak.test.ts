import { describe, expect, it } from 'vitest'
import { toggled } from './weak'

describe('toggled', () => {
  it('adds an item that is not in the list and removes one that is', () => {
    expect(toggled([], 'a')).toEqual(['a'])
    expect(toggled(['a', 'b'], 'a')).toEqual(['b'])
  })
  it('leaves the original list alone', () => {
    const list = ['a']
    toggled(list, 'b')
    expect(list).toEqual(['a'])
  })
})
