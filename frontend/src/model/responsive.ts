/**
 * Below this width the side panels become overlays and start closed.
 * Keep in step with the max-width media query in styles.css.
 */
export const NARROW_PX = 980

export const isNarrow = (): boolean => typeof window !== 'undefined' && window.innerWidth < NARROW_PX
