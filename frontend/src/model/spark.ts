/** Pixel coordinates for a sparkline: oldest value at x = 0, newest at x = width, larger values nearer the top. */
export function sparkPoints(values: number[], box: { width: number; height: number; max?: number }): [number, number][] {
  if (!values.length) return []
  const max = box.max ?? Math.max(...values, 0)
  const y = (v: number) => (max > 0 ? box.height - (Math.min(max, Math.max(0, v)) / max) * box.height : box.height)
  if (values.length === 1) return [[0, y(values[0])]]
  return values.map((v, i) => [(i / (values.length - 1)) * box.width, y(v)])
}

/** A sparkline in words for screen readers. */
export function sparkSummary(label: string, values: number[], fmt: (v: number) => string): string {
  if (!values.length) return `${label}: no data yet.`
  return `${label}: now ${fmt(values[values.length - 1])}, peak ${fmt(Math.max(...values))}, over the last ${values.length} s.`
}
