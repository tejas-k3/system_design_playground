import { describe, expect, it } from 'vitest'
import css from '../styles.css?raw'

// Text colours must stay readable (WCAG AA, 4.5:1) on every surface they are drawn on, in both themes.

function vars(selector: RegExp): Record<string, string> {
  const block = css.match(selector)?.[1] ?? ''
  return Object.fromEntries([...block.matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{6})/g)].map((m) => [m[1], m[2]]))
}
const light = vars(/:root\s*\{([^}]*)\}/)
const dark = { ...light, ...vars(/:root\[data-theme='dark'\]\s*\{([^}]*)\}/) }

const channel = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => channel(parseInt(hex.slice(i, i + 2), 16) / 255))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}
function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

describe.each([['light', light], ['dark', dark]] as const)('%s theme text contrast', (_name, v) => {
  for (const fg of ['ink', 'muted', 'accent', 'ok', 'warn', 'bad']) {
    for (const bg of ['bg', 'panel', 'panel2']) {
      it(`${fg} on ${bg} is at least 4.5:1`, () => {
        expect(v[fg], `--${fg} defined`).toBeTruthy()
        expect(contrast(v[fg], v[bg])).toBeGreaterThanOrEqual(4.5)
      })
    }
  }
})
