import { afterEach, describe, expect, it, vi } from 'vitest'
import { NARROW_PX, isNarrow } from './responsive'

afterEach(() => vi.unstubAllGlobals())

describe('isNarrow', () => {
  it('is false where there is no window to measure', () => {
    expect(isNarrow()).toBe(false)
  })

  it('switches exactly at the breakpoint the stylesheet uses', () => {
    vi.stubGlobal('window', { innerWidth: NARROW_PX - 1 })
    expect(isNarrow()).toBe(true)
    vi.stubGlobal('window', { innerWidth: NARROW_PX })
    expect(isNarrow()).toBe(false)
  })
})
