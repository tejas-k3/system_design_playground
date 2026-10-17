import type { LatencyBucket } from '../api/types'

const OVERFLOW = 1e9

const ms = (v: number) => (v >= 1000 ? `${+(v / 1000).toFixed(1)} s` : `${v} ms`)

/** Label of bar i: '≤ 100 ms', '200-500 ms', '1-2 s', '> 10 s'. */
export function bucketLabel(buckets: LatencyBucket[], i: number): string {
  const b = buckets[i]
  if (b.leMs >= OVERFLOW) return `> ${ms(buckets[i - 1]?.leMs ?? 0)}`
  if (i === 0) return `≤ ${ms(b.leMs)}`
  const lo = buckets[i - 1].leMs
  const unit = (v: number) => (v >= 1000 ? `${+(v / 1000).toFixed(1)}` : `${v}`)
  const both = lo >= 1000 || b.leMs < 1000
  return both ? `${unit(lo)}-${ms(b.leMs)}` : `${ms(lo)}-${ms(b.leMs)}`
}

const total = (buckets: LatencyBucket[]) => buckets.reduce((n, b) => n + b.count, 0)

/** Cumulative share of successes up to and including each bucket. */
export function cdf(buckets: LatencyBucket[]): number[] {
  const n = total(buckets)
  let run = 0
  return buckets.map((b) => ((run += b.count), n ? run / n : 0))
}

/** Index of the bucket holding quantile q (0..1), or -1 with no data. */
export function quantileBucket(buckets: LatencyBucket[], q: number): number {
  const c = cdf(buckets)
  if (!total(buckets)) return -1
  return c.findIndex((v) => v >= q - 1e-9)
}

/** The distribution in words, for screen readers. */
export function describeHistogram(buckets: LatencyBucket[]): string {
  const n = total(buckets)
  if (!n) return 'Latency distribution: no successful requests yet.'
  const c = cdf(buckets)
  const parts: string[] = []
  const last = buckets.length - 1
  for (let i = 0; i < last; i++) {
    if (buckets[i].count > 0 && c[i] < 1) parts.push(`${Math.round(c[i] * 100)}% up to ${ms(buckets[i].leMs)}`)
  }
  const tail = buckets[last].count
  const sentence = parts.length ? `${parts.join(', ')}${tail ? `, the slowest ${Math.round((tail / n) * 100)}% above ${ms(buckets[last - 1].leMs)}` : ''}.` : ''
  return `Latency distribution of ${n} successful requests. ${sentence}`.trim()
}
