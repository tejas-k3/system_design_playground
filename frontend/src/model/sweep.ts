import type { SweepPoint } from '../api/types'

/**
 * The knee of a load curve: the first load (after the lightest) where success falls under 99 % or
 * p95 reaches three times the p95 at the lightest load. Null if the design never bends in the range.
 */
export function findKnee(points: SweepPoint[]): number | null {
  if (points.length < 2) return null
  const base = points[0].p95
  for (let i = 1; i < points.length; i++) {
    if (points[i].successRate < 0.99 || (base > 0 && points[i].p95 >= 3 * base)) return i
  }
  return null
}

export function describeCurve(points: SweepPoint[], knee: number | null): string {
  if (points.length === 0) return 'No data yet.'
  if (knee === null) return `No knee up to ${Math.round(points[points.length - 1].rps)} requests per second.`
  const k = points[knee]
  return `Comfortable up to ${Math.round(points[knee - 1].rps)} requests per second; the knee is at ${Math.round(k.rps)} (success ${(k.successRate * 100).toFixed(1)}%, p95 ${Math.round(k.p95)} ms).`
}
