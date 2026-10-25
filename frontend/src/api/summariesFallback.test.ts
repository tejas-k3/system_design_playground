import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchScenariosOrCached } from './client'
import { SUMMARIES_KEY } from '../model/summaries'

const item = (id: string) => ({ id, title: `T ${id}`, difficulty: 'Easy', summary: 's' })
let mem: Map<string, string>

beforeEach(() => {
  mem = new Map()
  ;(globalThis as { window?: unknown }).window = { localStorage: { getItem: (k: string) => mem.get(k) ?? null, setItem: (k: string, v: string) => void mem.set(k, v) } }
})
afterEach(() => {
  vi.unstubAllGlobals()
  delete (globalThis as { window?: unknown }).window
})

describe('fetchScenariosOrCached', () => {
  it('returns the server list and keeps a copy', async () => {
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify([item('a'), item('b')])))
    const r = await fetchScenariosOrCached()
    expect(r.cached).toBe(false)
    expect(r.list).toHaveLength(2)
    expect(mem.has(SUMMARIES_KEY)).toBe(true)
  })

  it('falls back to the last good copy when the fetch fails', async () => {
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify([item('a')])))
    await fetchScenariosOrCached()
    vi.stubGlobal('fetch', async () => { throw new TypeError('Failed to fetch') })
    const r = await fetchScenariosOrCached()
    expect(r.cached).toBe(true)
    expect(r.list.map((s) => s.id)).toEqual(['a'])
  })

  it('falls back when the server answers an error, or too slowly', async () => {
    mem.set(SUMMARIES_KEY, JSON.stringify([item('z')]))
    vi.stubGlobal('fetch', async () => new Response('{"error":"boom"}', { status: 500 }))
    expect((await fetchScenariosOrCached()).cached).toBe(true)
    vi.stubGlobal('fetch', () => new Promise(() => undefined))
    expect((await fetchScenariosOrCached(20)).list[0].id).toBe('z')
  })

  it('does not trust a damaged copy: the failure is passed on', async () => {
    mem.set(SUMMARIES_KEY, '[{"id":1}]')
    vi.stubGlobal('fetch', async () => { throw new TypeError('Failed to fetch') })
    await expect(fetchScenariosOrCached()).rejects.toThrow('Failed to fetch')
  })

  it('does not overwrite the copy with a bad answer', async () => {
    mem.set(SUMMARIES_KEY, JSON.stringify([item('keep')]))
    vi.stubGlobal('fetch', async () => new Response('[]'))
    await fetchScenariosOrCached()
    expect(JSON.parse(mem.get(SUMMARIES_KEY)!)[0].id).toBe('keep')
  })
})
