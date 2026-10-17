import type { NodeStats } from '../api/types'

/** The chip text for something in progress on a component (a failover step with its countdown, or servers booting), or null. */
export function phaseChip(s: NodeStats | undefined): string | null {
  if (!s) return null
  if (s.phase) return `${s.phase === 'detecting' ? 'NOTICING' : 'PROMOTING'} ${Math.max(1, Math.ceil(s.phaseLeftSec ?? 0))} s`
  if (s.booting) return `${s.booting} booting`
  return null
}
