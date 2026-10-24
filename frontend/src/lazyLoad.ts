/** Pure helpers for loading deferred UI chunks: retry once, and schedule preloads for idle time. */

export function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}

/** Run `load`; if it rejects, wait `waitMs` and try exactly once more. */
export async function loadWithRetry<T>(load: () => Promise<T>, waitMs = 400): Promise<T> {
  try {
    return await load()
  } catch {
    await delay(waitMs)
    return load()
  }
}

export interface IdleEnv {
  requestIdleCallback?: (cb: () => void, opts?: { timeout: number }) => unknown
  setTimeout: (cb: () => void, ms: number) => unknown
}

/** Run `cb` when the browser is idle, or after `fallbackMs` where requestIdleCallback is missing. */
export function whenIdle(cb: () => void, env: IdleEnv, timeoutMs = 4000, fallbackMs = 1500): void {
  if (typeof env.requestIdleCallback === 'function') env.requestIdleCallback(cb, { timeout: timeoutMs })
  else env.setTimeout(cb, fallbackMs)
}

/** Preload the given loaders one after another, each in its own idle slot; failures are ignored (the open path retries). */
export function preloadSequentially(loaders: Array<() => Promise<unknown>>, env: IdleEnv): void {
  const rest = [...loaders]
  const next = () => {
    const l = rest.shift()
    if (!l) return
    whenIdle(() => { l().catch(() => {}).finally(next) }, env)
  }
  next()
}
