export interface Palette {
  bg: string
  grid: string
  ink: string
  muted: string
  card: string
  accent: string
  ok: string
  warn: string
  bad: string
  noteBg: string
}

export const palettes: Record<'light' | 'dark', Palette> = {
  light: { bg: '#fbfaf6', grid: '#dedad0', ink: '#1f1f1f', muted: '#6c6a63', card: '#ffffff', accent: '#3b5bdb', ok: '#237a37', warn: '#e67700', bad: '#c92a2a', noteBg: '#fff3bf' },
  dark: { bg: '#15161a', grid: '#2b2d33', ink: '#ececec', muted: '#9a9ca3', card: '#212329', accent: '#748ffc', ok: '#51cf66', warn: '#fcc419', bad: '#ff6b6b', noteBg: '#4a4220' },
}

export const FONT = '"Kalam", "Segoe Print", "Comic Sans MS", cursive'

export function utilColor(u: number, p: Palette): string {
  return u < 0.6 ? p.ok : u < 0.85 ? p.warn : p.bad
}
