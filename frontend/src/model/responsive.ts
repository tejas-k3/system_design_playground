/**
 * Layout tiers. Keep in step with the max-width media queries in styles.css.
 *  - wide    (>= COMPACT_PX): library, challenge panel and inspector are all docked beside the canvas.
 *  - compact (NARROW_PX to COMPACT_PX): the inspector is an overlay drawer that opens when something is
 *    selected, so the canvas keeps room (it is only ~150 px wide at 1024 px with everything docked).
 *  - narrow  (< NARROW_PX): phones. Panels start closed, the challenge is a strip above the board and
 *    the inspector is a bottom sheet.
 */
export const NARROW_PX = 980
export const COMPACT_PX = 1360 // library 216 + challenge 320 + inspector 300 leaves >= 480 px of canvas from here up

const width = (): number | null => (typeof window === 'undefined' ? null : window.innerWidth)

export const isNarrow = (): boolean => (width() ?? Infinity) < NARROW_PX
export const isCompact = (): boolean => (width() ?? Infinity) < COMPACT_PX
