import { describe, expect, it } from 'vitest'
import { formatEstimate, parseNumber, verdictLabel } from './questions'

describe('parseNumber', () => {
  it('reads plain numbers, commas and decimals', () => {
    expect(parseNumber('2314')).toBe(2314)
    expect(parseNumber('2,314.8')).toBe(2314.8)
    expect(parseNumber(' 0.5 ')).toBe(0.5)
  })
  it('reads k, M, G and T suffixes in either case', () => {
    expect(parseNumber('1.5k')).toBe(1500)
    expect(parseNumber('2M')).toBe(2_000_000)
    expect(parseNumber('3 g')).toBe(3e9)
    expect(parseNumber('1T')).toBe(1e12)
  })
  it('reads scientific notation', () => {
    expect(parseNumber('3e6')).toBe(3_000_000)
  })
  it('refuses empty text, words, and non-positive numbers', () => {
    for (const bad of ['', '  ', 'abc', '12x', '-4', '0', 'k', '1..2']) expect(parseNumber(bad)).toBeNull()
  })
})

describe('formatEstimate', () => {
  it('uses thousands separators for big numbers and keeps small ones precise', () => {
    expect(formatEstimate(23148.148)).toBe('23,148')
    expect(formatEstimate(38.58)).toBe('38.6')
    expect(formatEstimate(0.926)).toBe('0.93')
    expect(formatEstimate(3)).toBe('3')
  })
})

describe('verdictLabel', () => {
  it('says it in words', () => {
    expect(verdictLabel('good')).toBe('Good: within 30%')
    expect(verdictLabel('close')).toBe('Close: within 2×')
    expect(verdictLabel('off')).toBe('Off by more than 2×')
  })
})

import { MODE_FILTERS, modeOfQuestion, modeOfScenario, questionModeLabel, showsMode } from './questions'

describe('mode filter', () => {
  it('knows the mode of a challenge and of a question', () => {
    expect(modeOfScenario({ open: true })).toBe('open')
    expect(modeOfScenario({})).toBe('fix')
    expect(modeOfQuestion({ mode: 'tradeoff' })).toBe('tradeoff')
  })
  it('shows everything under All and only the chosen mode otherwise', () => {
    expect(showsMode('all', 'review')).toBe(true)
    expect(showsMode('review', 'review')).toBe(true)
    expect(showsMode('review', 'fix')).toBe(false)
  })
  it('names every filter in words', () => {
    expect(MODE_FILTERS.map((f) => f.label)).toEqual(['All', 'Fix a design', 'Build from scratch', 'Warm-up', 'Requirements', 'Estimate', 'Diagnose', 'Trade-off', 'Review'])
  })
  it('labels a question by its kind on the card', () => {
    expect(questionModeLabel('match')).toBe('warm-up')
    expect(questionModeLabel('ask')).toBe('requirements')
    expect(questionModeLabel('tradeoff')).toBe('trade-off')
    expect(questionModeLabel('estimate')).toBe('estimate')
  })
  it('files a matching question under the warm-up filter', () => {
    expect(showsMode('match', modeOfQuestion({ mode: 'match' }))).toBe(true)
    expect(showsMode('estimate', modeOfQuestion({ mode: 'match' }))).toBe(false)
  })
})
