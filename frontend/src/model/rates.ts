/** Compact request rate: 7.3, 523, 1.2k, 12k. */
export function fmtRate(v: number): string {
  if (v >= 10000) return `${(v / 1000).toFixed(0)}k`
  if (v >= 1000) return `${(v / 1000).toFixed(1)}k`
  return v >= 10 ? `${Math.round(v)}` : v.toFixed(1)
}

/** Subtitle of a Users node: the configured rate at rest, the rate really being sent while running
 *  (a challenge's traffic curve overrides the node's own setting, so the setting alone would mislead). */
export function clientRateLabel(rp: { rps: number; pattern: string; peakRps: number }, running: boolean, sentRps: number): string {
  if (running && sentRps >= 0.5) return `now ${fmtRate(sentRps)}/s`
  return `${rp.rps} rps${rp.pattern !== 'constant' ? ` → ${rp.peakRps}` : ''}`
}

/** Label on a connection: the live rate while running, plus the latency when it is selected or hovered. */
export function edgeLabel(latencyMs: number, rps: number, detailed: boolean, running: boolean): string {
  const parts: string[] = []
  if (running && rps >= 0.5) parts.push(`${fmtRate(rps)}/s`)
  if (detailed) parts.push(`${latencyMs} ms`)
  return parts.join(' · ')
}

/** Filled width of a utilisation bar: nothing when idle (an idle component must not look busy), at least a sliver once busy. */
export function utilBarWidth(util: number, full: number): number {
  return util <= 0 ? 0 : Math.max(4, full * Math.min(1, util))
}
