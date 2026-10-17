import { describe, expect, it } from 'vitest'
import { clientRateLabel, edgeLabel, serverSubtitle, smoothRate, utilBarWidth } from './rates'

describe('clientRateLabel', () => {
  it('shows the configured rate before a run, with the peak for a pattern', () => {
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, false, 0)).toBe('80 rps')
    expect(clientRateLabel({ rps: 80, pattern: 'spike', peakRps: 600 }, false, 0)).toBe('80 rps → 600')
  })
  it('shows what is actually being sent while running, since challenges can drive the rate with a curve', () => {
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, true, 523.4)).toBe('now 523/s')
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, true, 7.25)).toBe('now 7.3/s')
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, true, 1234)).toBe('now 1.2k/s')
  })
  it('previews a challenge curve at rest instead of the node setting it overrides', () => {
    const curve: [number, number][] = [[0, 80], [8, 80], [14, 600], [50, 600], [56, 80]]
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, false, 0, curve)).toBe('80 → 600 rps')
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, false, 0, [[0, 100], [10, 100]])).toBe('100 rps')
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, true, 580, curve)).toBe('now 580/s')
  })
  it('falls back to the configured rate while the first numbers are still arriving', () => {
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, true, 0)).toBe('80 rps')
  })
})

describe('edgeLabel', () => {
  it('shows the live rate while running', () => {
    expect(edgeLabel(5, 580, false, true)).toBe('580/s')
    expect(edgeLabel(5, 0.2, false, true)).toBe('')
  })
  it('adds the latency when the connection is selected or hovered, so the label never silently changes meaning', () => {
    expect(edgeLabel(5, 580, true, true)).toBe('580/s · 5 ms')
    expect(edgeLabel(5, 0, true, true)).toBe('5 ms')
    expect(edgeLabel(5, 0, true, false)).toBe('5 ms')
  })
  it('shows nothing at rest unless selected', () => {
    expect(edgeLabel(5, 0, false, false)).toBe('')
  })
})

describe('utilBarWidth', () => {
  it('is empty for an idle component and never invisible for a busy one', () => {
    expect(utilBarWidth(0, 100)).toBe(0)
    expect(utilBarWidth(0.01, 100)).toBe(4)
    expect(utilBarWidth(0.5, 100)).toBe(50)
    expect(utilBarWidth(3, 100)).toBe(100)
  })
})

describe('smoothRate', () => {
  it('starts at the first value', () => {
    expect(smoothRate(undefined, 120, 16)).toBe(120)
  })
  it('moves toward the target without jumping', () => {
    const next = smoothRate(100, 200, 16)
    expect(next).toBeGreaterThan(100)
    expect(next).toBeLessThan(130)
  })
  it('settles on the target given time, and tolerates a long pause', () => {
    let v = 100
    for (let i = 0; i < 120; i++) v = smoothRate(v, 200, 16)
    expect(v).toBeCloseTo(200, 0)
    expect(smoothRate(100, 200, 5000)).toBeCloseTo(200, 0)
  })
})

describe('serverSubtitle', () => {
  it('is the thread count of one server', () => {
    expect(serverSubtitle(8)).toBe('8 threads')
    expect(serverSubtitle(8, 0)).toBe('8 threads')
    expect(serverSubtitle(8, 1)).toBe('8 threads')
  })
  it('multiplies by the servers an autoscaled node is running', () => {
    expect(serverSubtitle(8, 3)).toBe('8 threads × 3')
  })
})
