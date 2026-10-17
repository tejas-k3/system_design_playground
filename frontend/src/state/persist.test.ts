import { describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import { parseSaved } from './persist'

describe('parseSaved', () => {
  it('ignores missing, malformed and non-object input', () => {
    expect(parseSaved(null, reg)).toEqual({})
    expect(parseSaved('{not json', reg)).toEqual({})
    expect(parseSaved('[1,2]', reg)).toEqual({})
    expect(parseSaved('"hi"', reg)).toEqual({})
  })

  it('keeps valid fields and drops invalid ones', () => {
    const out = parseSaved(
      JSON.stringify({
        theme: 'purple',
        progress: { a: 3, b: 7, c: 'x', d: -1, e: 0 },
        viewport: { scrollX: 5, scrollY: 6, zoom: 99 },
        sandbox: { nodes: [{ id: 'n', kind: 'client', x: 1, y: 2, params: { rps: 99999 } }], edges: [], notes: [] },
      }),
      reg,
    )
    expect(out.theme).toBeUndefined()
    expect(out.progress).toEqual({ a: 3, e: 0 })
    expect(out.viewport).toEqual({ scrollX: 5, scrollY: 6, zoom: 4 })
    expect(out.sandbox?.nodes[0].params).toEqual({ rps: 5000 })
  })

  it('rejects a viewport with non-finite numbers', () => {
    expect(parseSaved(JSON.stringify({ viewport: { scrollX: null, scrollY: 1, zoom: 1 } }), reg).viewport).toBeUndefined()
  })
})
