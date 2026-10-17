import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from '../state/store'
import { reg } from '../test/fixtures'
import { openDesign, shareCurrentDesign } from './designs'

const st = () => useStore.getState()
const design = { nodes: [{ id: 'n', kind: 'appServer', x: 1, y: 2, label: 'Shared', params: {} }], edges: [], notes: [] }

function mockFetch(handler: (url: string, init?: RequestInit) => { status: number; body: unknown }) {
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const r = handler(url, init)
    return { ok: r.status < 400, status: r.status, statusText: 'x', json: async () => r.body }
  }))
}

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true)
  st().setReg(reg)
  vi.stubGlobal('location', { origin: 'https://sdp.test' })
})
afterEach(() => vi.unstubAllGlobals())

describe('shareCurrentDesign', () => {
  it('refuses to share an empty design', async () => {
    mockFetch(() => ({ status: 201, body: { id: 'x' } }))
    await shareCurrentDesign()
    expect(fetch).not.toHaveBeenCalled()
    expect(st().toasts.at(-1)?.kind).toBe('warn')
  })

  it('saves the design and copies its link', async () => {
    st().addNode('appServer', 0, 0)
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    mockFetch(() => ({ status: 201, body: { id: '3ibupvsc57zfg' } }))
    await shareCurrentDesign()
    expect(writeText).toHaveBeenCalledWith('https://sdp.test/#design=3ibupvsc57zfg')
    const call = vi.mocked(fetch).mock.calls[0]
    expect(call[0]).toBe('/api/designs')
    expect(JSON.parse(String((call[1] as RequestInit).body)).diagram.nodes).toHaveLength(1)
  })

  it('shows the link instead when the clipboard is blocked', async () => {
    st().addNode('appServer', 0, 0)
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    mockFetch(() => ({ status: 201, body: { id: '3ibupvsc57zfg' } }))
    await shareCurrentDesign()
    expect(st().toasts.at(-1)?.text).toContain('https://sdp.test/#design=3ibupvsc57zfg')
  })

  it('still shows the link when the clipboard never answers', async () => {
    vi.useFakeTimers()
    try {
      st().addNode('appServer', 0, 0)
      vi.stubGlobal('navigator', { clipboard: { writeText: () => new Promise(() => {}) } }) // never settles
      mockFetch(() => ({ status: 201, body: { id: '3ibupvsc57zfg' } }))
      const done = shareCurrentDesign()
      await vi.advanceTimersByTimeAsync(2000)
      await done
      expect(st().toasts.at(-1)?.text).toContain('Share this link: https://sdp.test/#design=3ibupvsc57zfg')
    } finally {
      vi.useRealTimers()
    }
  })

  it('reports a server refusal', async () => {
    st().addNode('appServer', 0, 0)
    mockFetch(() => ({ status: 429, body: { error: 'too many designs shared, try again in a minute' } }))
    await shareCurrentDesign()
    expect(st().toasts.at(-1)).toMatchObject({ kind: 'error' })
    expect(st().toasts.at(-1)?.text).toContain('too many designs')
  })
})

describe('openDesign', () => {
  it('opens the design and keeps the previous one one undo away', async () => {
    st().addNode('database', 0, 0)
    mockFetch(() => ({ status: 200, body: { diagram: design } }))
    await openDesign('3ibupvsc57zfg')
    expect(st().diagram.nodes.map((n) => n.label)).toEqual(['Shared'])
    st().undo()
    expect(st().diagram.nodes.map((n) => n.kind)).toEqual(['database'])
  })

  it('sanitizes what the server returns', async () => {
    mockFetch(() => ({ status: 200, body: { diagram: { ...design, nodes: [...design.nodes, { id: 'bad', kind: 'nope', x: 0, y: 0 }] } } }))
    await openDesign('3ibupvsc57zfg')
    expect(st().diagram.nodes).toHaveLength(1)
  })

  it('leaves the current design alone when the link is unknown or the design is empty', async () => {
    st().addNode('database', 0, 0)
    mockFetch(() => ({ status: 404, body: { error: 'no such design' } }))
    await openDesign('aaaaaaaaaaaaa')
    expect(st().diagram.nodes.map((n) => n.kind)).toEqual(['database'])
    expect(st().toasts.at(-1)?.kind).toBe('error')

    mockFetch(() => ({ status: 200, body: { diagram: { nodes: [], edges: [], notes: [] } } }))
    await openDesign('aaaaaaaaaaaaa')
    expect(st().diagram.nodes).toHaveLength(1)
  })
})
