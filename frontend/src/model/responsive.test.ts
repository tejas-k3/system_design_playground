import { afterEach, describe, expect, it, vi } from 'vitest'
import { COMPACT_PX, NARROW_PX, isCompact, isNarrow } from './responsive'

afterEach(() => vi.unstubAllGlobals())
const at = (innerWidth: number) => vi.stubGlobal('window', { innerWidth })

describe('layout tiers', () => {
  it('treats a missing window as the widest layout', () => {
    expect(isNarrow()).toBe(false)
    expect(isCompact()).toBe(false)
  })

  it('switches exactly at the breakpoints the stylesheet uses', () => {
    at(NARROW_PX - 1)
    expect([isNarrow(), isCompact()]).toEqual([true, true])
    at(NARROW_PX)
    expect([isNarrow(), isCompact()]).toEqual([false, true])
    at(COMPACT_PX - 1)
    expect([isNarrow(), isCompact()]).toEqual([false, true])
    at(COMPACT_PX)
    expect([isNarrow(), isCompact()]).toEqual([false, false])
  })

  it('keeps the tiers nested: narrow screens are always compact too', () => {
    expect(NARROW_PX).toBeLessThan(COMPACT_PX)
  })

  it('uses the overlay drawer only where a docked inspector would squeeze the canvas below ~480 px', () => {
    for (const w of [1024, 1280]) {
      at(w)
      expect(isCompact()).toBe(true)
    }
    for (const w of [1366, 1440, 1536, 1920]) {
      at(w)
      expect(isCompact()).toBe(false)
    }
  })
})
