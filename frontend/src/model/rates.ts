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
