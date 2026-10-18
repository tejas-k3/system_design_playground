import { describe, expect, it } from 'vitest'
import type { ParamSpec } from '../api/types'
import { parseValues, suggestValues } from './whatif'

const spec = (over: Partial<ParamSpec> = {}): ParamSpec => ({ key: 'threads', label: 'Threads', group: 'g', type: 'number', default: 8, min: 1, max: 64, step: 1, ...over })

describe('suggestValues', () => {
  it('spreads values around the current one within the range, whole numbers for a step of 1', () => {
    expect(suggestValues(spec(), 8)).toEqual([2, 4, 8, 16, 32])
  })
  it('stays inside min and max and has no repeats', () => {
    expect(suggestValues(spec({ min: 1, max: 10 }), 8)).toEqual([2, 4, 8, 10])
    expect(suggestValues(spec({ min: 1, max: 2 }), 1)).toEqual([1, 2])
  })
  it('keeps decimals for fractional steps', () => {
    expect(suggestValues(spec({ min: 0, max: 1, step: 0.1 }), 0.4)).toEqual([0.1, 0.2, 0.4, 0.8, 1]) // 1.6 is clamped to the maximum
  })
})

describe('parseValues', () => {
  it('reads comma or space separated numbers, sorted and de-duplicated', () => {
    expect(parseValues('16, 4 8,4')).toEqual({ values: [4, 8, 16], error: '' })
  })
  it('refuses junk, empty input and more than 20 values', () => {
    expect(parseValues('4, x').error).toMatch(/number/i)
    expect(parseValues('  ').error).toMatch(/at least one/i)
    expect(parseValues(Array.from({ length: 21 }, (_, i) => i + 1).join(',')).error).toMatch(/20/)
  })
})
