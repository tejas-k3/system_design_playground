import { describe, expect, it } from 'vitest'
import { llmTiles } from './llmTiles'

describe('LLM metric tiles', () => {
  it('shows nothing for a design without a model', () => {
    expect(llmTiles({})).toEqual([])
    expect(llmTiles({ ttftMs: 0, itlMs: 0 })).toEqual([])
  })
  it('shows the first token and the token gap, rounded', () => {
    const t = llmTiles({ ttftMs: 432.4, itlMs: 72.6 })
    expect(t.map((x) => [x.label, x.value])).toEqual([['First token', '432 ms'], ['Token gap', '73 ms']])
  })
  it('shows a dash for the gap when no answer has more than one token yet', () => {
    expect(llmTiles({ ttftMs: 100 })[1].value).toBe('—')
  })
})
