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
