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

import { emptyInterview } from '../model/interview'

describe('saved data, version 1 to 2', () => {
  const v1 = {
    version: 1,
    sandbox: { nodes: [{ id: 'u', kind: 'client', x: 1, y: 2, label: 'Users', params: { rps: 80 } }, { id: 'a', kind: 'appServer', x: 5, y: 6, label: 'App', params: { threads: 12 } }], edges: [{ id: 'u>a', from: 'u', to: 'a', latencyMs: 40 }], notes: [] },
    theme: 'dark',
    progress: { 'hello-world': 3, 'slow-reads': 1 },
    viewport: { scrollX: 10, scrollY: 20, zoom: 1.5 },
  }

  it('a version 1 blob loads exactly as before and has no interview', () => {
    const out = parseSaved(JSON.stringify(v1), reg)
    expect(out.sandbox?.nodes.map((n) => n.id)).toEqual(['u', 'a'])
    expect(out.sandbox?.nodes[1].params).toEqual({ threads: 12 })
    expect(out.sandbox?.edges).toHaveLength(1)
    expect(out.theme).toBe('dark')
    expect(out.progress).toEqual({ 'hello-world': 3, 'slow-reads': 1 })
    expect(out.viewport).toEqual({ scrollX: 10, scrollY: 20, zoom: 1.5 })
    expect(out.interview).toBeUndefined()
  })

  it('a version 2 blob carries the interview as well', () => {
    const out = parseSaved(JSON.stringify({ ...v1, version: 2, interview: { ...emptyInterview(), notes: 'hello' } }), reg)
    expect(out.interview?.notes).toBe('hello')
    expect(out.progress).toEqual({ 'hello-world': 3, 'slow-reads': 1 })
  })

  it('a damaged interview is dropped without costing the diagram or the stars', () => {
    const out = parseSaved(JSON.stringify({ ...v1, version: 2, interview: { api: 'nope' } }), reg)
    expect(out.interview).toBeUndefined()
    expect(out.sandbox?.nodes).toHaveLength(2)
    expect(out.progress).toEqual({ 'hello-world': 3, 'slow-reads': 1 })
    expect(out.theme).toBe('dark')
  })

  it('writes version 2 under the same storage key, so nothing is orphaned', async () => {
    const mod = await import('./persist')
    expect(mod.STORAGE_KEY).toBe('sdp:v1')
    expect(mod.SAVED_VERSION).toBe(2)
  })
})
