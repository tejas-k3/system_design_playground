import { describe, expect, it } from 'vitest'
import { scrubTarget } from './scrub'

describe('scrubTarget', () => {
  it('steps one second with the arrow keys and ten with Shift', () => {
    expect(scrubTarget(60, 30, 'ArrowLeft', false)).toBe(29)
    expect(scrubTarget(60, 30, 'ArrowRight', false)).toBe(31)
    expect(scrubTarget(60, 30, 'ArrowLeft', true)).toBe(20)
    expect(scrubTarget(60, 30, 'ArrowRight', true)).toBe(40)
  })
  it('also honours Down/Up like a slider, Home and End', () => {
    expect(scrubTarget(60, 30, 'ArrowDown', false)).toBe(29)
    expect(scrubTarget(60, 30, 'ArrowUp', false)).toBe(31)
    expect(scrubTarget(60, 30, 'Home', false)).toBe(0)
    expect(scrubTarget(60, 30, 'End', false)).toBe(60)
  })
  it('never leaves 0..now', () => {
    expect(scrubTarget(60, 3, 'ArrowLeft', true)).toBe(0)
    expect(scrubTarget(60, 58, 'ArrowRight', true)).toBe(60)
  })
  it('leaves other keys alone', () => {
    expect(scrubTarget(60, 30, 'a', false)).toBeNull()
  })
})
