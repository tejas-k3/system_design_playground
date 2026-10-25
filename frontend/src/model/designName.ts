import type { Diagram } from '../api/types'

export const DEFAULT_DESIGN_NAME = 'Untitled design'
export const MAX_DESIGN_NAME = 60
export const APP_TITLE = 'System Design Playground'

/**
 * What is kept of a typed design name: control characters removed, trimmed, cut to 60 characters (not bytes) and
 * trimmed again. An empty result means "no name"; the default is applied only when showing it. Mirrors the server's
 * sanitiser, so a name round-trips unchanged.
 */
export function normalizeDesignName(raw: string): string {
  // eslint-disable-next-line no-control-regex
  const clean = raw.replace(/[\u0000-\u001f\u007f-\u009f]/g, '').trim()
  return [...clean].slice(0, MAX_DESIGN_NAME).join('').trim()
}

/** The name to show for a design: its own, or "Untitled design". */
export const designName = (d: Pick<Diagram, 'name'>): string => d.name || DEFAULT_DESIGN_NAME

/** The browser tab title. */
export const pageTitle = (name: string): string => `${name} · ${APP_TITLE}`
