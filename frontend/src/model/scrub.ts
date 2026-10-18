/** Where a key moves the rewind slider, in simulated seconds, or null for keys it does not handle. */
export function scrubTarget(now: number, current: number, key: string, shift: boolean): number | null {
  const step = shift ? 10 : 1
  let t: number
  switch (key) {
    case 'ArrowLeft':
    case 'ArrowDown':
      t = current - step
      break
    case 'ArrowRight':
    case 'ArrowUp':
      t = current + step
      break
    case 'Home':
      t = 0
      break
    case 'End':
      t = now
      break
    default:
      return null
  }
  return Math.min(now, Math.max(0, t))
}
