import type { Node } from '../api/types'

/** Where a component runs, as text: its region and zone when the design sets them ("eu · az-b"); nothing otherwise. */
export function placementTag(n: Pick<Node, 'params'>): string {
  const region = typeof n.params.region === 'string' ? n.params.region : ''
  const zone = typeof n.params.zone === 'string' ? n.params.zone : ''
  return [region, zone].filter(Boolean).join(' · ')
}
