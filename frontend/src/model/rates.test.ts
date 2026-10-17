import { describe, expect, it } from 'vitest'
import { clientRateLabel } from './rates'

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
  it('falls back to the configured rate while the first numbers are still arriving', () => {
    expect(clientRateLabel({ rps: 80, pattern: 'constant', peakRps: 80 }, true, 0)).toBe('80 rps')
  })
})
