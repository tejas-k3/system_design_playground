/** Compact request rate: 7.3, 523, 1.2k, 12k. */
export function fmtRate(v: number): string {
  if (v >= 10000) return `${(v / 1000).toFixed(0)}k`
  if (v >= 1000) return `${(v / 1000).toFixed(1)}k`
  return v >= 10 ? `${Math.round(v)}` : v.toFixed(1)
}

/** Subtitle of a Users node: the configured rate at rest, the rate really being sent while running
 *  (a challenge's traffic curve overrides the node's own setting, so the setting alone would mislead). */
export function clientRateLabel(
  rp: { rps: number; pattern: string; peakRps: number; arrival?: string; users?: number; thinkMs?: number },
  running: boolean,
  sentRps: number,
  curve?: [number, number][],
): string {
  if (running && sentRps >= 0.5) return `now ${fmtRate(sentRps)}/s`
  if (rp.arrival === 'closed') return `${rp.users ?? 0} users · ${+((rp.thinkMs ?? 0) / 1000).toFixed(1)} s think`
  if (curve && curve.length > 0) {
    const rates = curve.map((p) => p[1])
    const [lo, hi] = [Math.min(...rates), Math.max(...rates)]
    return lo === hi ? `${Math.round(lo)} rps` : `${Math.round(lo)} → ${Math.round(hi)} rps`
  }
  return `${rp.rps} rps${rp.pattern !== 'constant' ? ` → ${rp.peakRps}` : ''}${rp.arrival === 'bursty' ? ' · bursty' : ''}`
}

/** Label on a connection: the live rate while running, plus the latency when it is selected or hovered. */
export interface LinkInfo {
  mbps?: number // data carried over the last seconds
  util?: number // share of the limit, if there is one
  limit?: number // the bandwidth limit, if there is one
  protocol?: string // a label on the connection
  async?: boolean // a fire-and-forget connection
}

const FULL = 0.95
const fmtMbps = (m: number) => (m >= 1000 ? `${Math.round(m / 100) / 10} Gbit/s` : `${Math.round(m)} Mbit/s`)

export function edgeLabel(latencyMs: number, rps: number, detailed: boolean, running: boolean, link?: LinkInfo): string {
  const parts: string[] = []
  if (link?.protocol) parts.push(link.protocol)
  if (link?.async) parts.push('async')
  if (running && rps >= 0.5) parts.push(`${fmtRate(rps)}/s`)
  if (running && link?.mbps && link.mbps >= 0.5) parts.push(fmtMbps(link.mbps) + (link.util !== undefined && link.util >= FULL ? ' FULL' : ''))
  if (detailed) parts.push(`${latencyMs} ms`)
  if (detailed && link?.limit) parts.push(`limit ${fmtMbps(link.limit)}`)
  return parts.join(' · ')
}

/** Filled width of a utilisation bar: nothing when idle (an idle component must not look busy), at least a sliver once busy. */
export function utilBarWidth(util: number, full: number): number {
  return util <= 0 ? 0 : Math.max(4, full * Math.min(1, util))
}

/** Eases a displayed number toward its latest value (time constant about 300 ms) so a rate label glides instead of flickering. */
export function smoothRate(shown: number | undefined, target: number, dtMs: number): number {
  if (shown === undefined) return target
  return shown + (target - shown) * (1 - Math.exp(-dtMs / 300))
}

/** Subtitle of an app server: its threads, times the number of servers when it is autoscaled and running several. */
export function serverSubtitle(threads: number, instances?: number): string {
  return instances && instances > 1 ? `${threads} threads × ${instances}` : `${threads} threads`
}
