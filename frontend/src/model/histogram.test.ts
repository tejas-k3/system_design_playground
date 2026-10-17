import { describe, expect, it } from 'vitest'
import type { LatencyBucket } from '../api/types'
import { bucketLabel, cdf, describeHistogram, quantileBucket } from './histogram'

const b = (leMs: number, count: number): LatencyBucket => ({ leMs, count })
const sample: LatencyBucket[] = [b(100, 50), b(200, 40), b(500, 8), b(1e9, 2)]

describe('bucketLabel', () => {
  it('writes milliseconds and seconds the way the rest of the UI does', () => {
    expect(bucketLabel(sample, 0)).toBe('≤ 100 ms')
    expect(bucketLabel(sample, 2)).toBe('200-500 ms')
    expect(bucketLabel([b(500, 1), b(1000, 1), b(2000, 1), b(1e9, 1)], 2)).toBe('1-2 s')
    expect(bucketLabel(sample, 3)).toBe('> 500 ms')
  })
})

describe('cdf', () => {
  it('is the running share of successes, ending at 1', () => {
    expect(cdf(sample)).toEqual([0.5, 0.9, 0.98, 1])
  })
  it('is all zeros for no data rather than NaN', () => {
    expect(cdf([b(100, 0), b(1e9, 0)])).toEqual([0, 0])
  })
})

describe('quantileBucket', () => {
  it('finds the bucket that holds the given quantile', () => {
    expect(quantileBucket(sample, 0.5)).toBe(0)
    expect(quantileBucket(sample, 0.95)).toBe(2)
    expect(quantileBucket(sample, 0.99)).toBe(3)
  })
  it('is -1 with no data', () => {
    expect(quantileBucket([b(100, 0)], 0.5)).toBe(-1)
  })
})

describe('describeHistogram', () => {
  it('summarises the distribution in words for screen readers', () => {
    expect(describeHistogram(sample)).toBe('Latency distribution of 100 successful requests. 50% up to 100 ms, 90% up to 200 ms, 98% up to 500 ms, the slowest 2% above 500 ms.')
  })
  it('says there is nothing yet', () => {
    expect(describeHistogram([b(100, 0), b(1e9, 0)])).toBe('Latency distribution: no successful requests yet.')
  })
})
