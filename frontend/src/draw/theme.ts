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
  light: { bg: '#f6f1e7', grid: '#d9cfb8', ink: '#1d2433', muted: '#645f52', card: '#fffdf8', accent: '#b43a0a', ok: '#1f6e31', warn: '#e67700', bad: '#b72424', noteBg: '#ffe9a8' },
  dark: { bg: '#0e1a2b', grid: '#1d3350', ink: '#e8eefc', muted: '#9fb0cc', card: '#16273f', accent: '#ffa94d', ok: '#69db7c', warn: '#ffd43b', bad: '#ff8787', noteBg: '#4a4220' },
}

export const FONT = '"Patrick Hand", "Segoe Print", "Comic Sans MS", cursive'

export function utilColor(u: number, p: Palette): string {
  return u < 0.6 ? p.ok : u < 0.85 ? p.warn : p.bad
}
