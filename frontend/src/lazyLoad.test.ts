import { describe, expect, it, vi } from 'vitest'
import { loadWithRetry, preloadSequentially, whenIdle } from './lazyLoad'

describe('loadWithRetry', () => {
  it('returns the first success without retrying', async () => {
    const f = vi.fn().mockResolvedValue(1)
    expect(await loadWithRetry(f, 0)).toBe(1)
    expect(f).toHaveBeenCalledTimes(1)
  })
  it('retries once after a failure', async () => {
    const f = vi.fn().mockRejectedValueOnce(new Error('x')).mockResolvedValue(2)
    expect(await loadWithRetry(f, 0)).toBe(2)
    expect(f).toHaveBeenCalledTimes(2)
  })
  it('gives up after the second failure', async () => {
    const f = vi.fn().mockRejectedValue(new Error('x'))
    await expect(loadWithRetry(f, 0)).rejects.toThrow('x')
    expect(f).toHaveBeenCalledTimes(2)
  })
})

describe('whenIdle', () => {
  it('uses requestIdleCallback when present', () => {
    const ric = vi.fn()
    const st = vi.fn()
    whenIdle(() => {}, { requestIdleCallback: ric, setTimeout: st })
    expect(ric).toHaveBeenCalledTimes(1)
    expect(st).not.toHaveBeenCalled()
  })
  it('falls back to setTimeout', () => {
    const st = vi.fn()
    whenIdle(() => {}, { setTimeout: st }, 4000, 1234)
    expect(st).toHaveBeenCalledWith(expect.any(Function), 1234)
  })
})

describe('preloadSequentially', () => {
  it('runs loaders in order, one per idle slot, surviving failures', async () => {
    const order: string[] = []
    const env = { setTimeout: (cb: () => void) => { queueMicrotask(cb) } }
    const done = new Promise<void>((resolve) => {
      preloadSequentially([
        async () => { order.push('a') },
        async () => { order.push('b'); throw new Error('boom') },
        async () => { order.push('c'); resolve() },
      ], env)
    })
    await done
    expect(order).toEqual(['a', 'b', 'c'])
  })
})
