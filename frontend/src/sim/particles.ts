import type { Particle } from '../api/types'

// Frames arrive from the socket; the canvas drains them on its next animation frame.
let queue: Particle[] = []

export function pushParticles(p: Particle[] | undefined): void {
  if (!p?.length) return
  queue.push(...p)
  if (queue.length > 600) queue = queue.slice(-600)
}

export function drainParticles(): Particle[] {
  const q = queue
  queue = []
  return q
}

/** Spawn times for a burst of n particles that arrived together: spread evenly over the time since the previous frame
 *  (at most 120 ms back, so a long pause in a background tab does not smear them), the newest at `now`. A burst drawn all at
 *  once looks like clumps of dots; spread out, the stream looks continuous. */
export function spawnTimes(n: number, now: number, sinceLastMs: number): number[] {
  const span = Math.min(120, Math.max(16, sinceLastMs))
  return Array.from({ length: n }, (_, i) => now - span * (1 - (i + 1) / n))
}
