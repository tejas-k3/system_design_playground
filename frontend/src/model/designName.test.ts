import { describe, expect, it } from 'vitest'
import { DEFAULT_DESIGN_NAME, MAX_DESIGN_NAME, designName, normalizeDesignName, pageTitle } from './designName'
import { sanitizeDiagram } from './diagram'
import { reg } from '../test/fixtures'

describe('normalizeDesignName', () => {
  it('trims, and an empty or blank name becomes empty (the default is applied on display)', () => {
    expect(normalizeDesignName('  Checkout  ')).toBe('Checkout')
    expect(normalizeDesignName('')).toBe('')
    expect(normalizeDesignName(' \t\n ')).toBe('')
  })
  it('removes control characters but keeps ordinary text, emoji and accents', () => {
    expect(normalizeDesignName('a\u0000b\tc\nd\u007fe')).toBe('abcde')
    expect(normalizeDesignName('Café 🚀 plan')).toBe('Café 🚀 plan')
  })
  it('is cut to 60 characters, counting characters not bytes, and trimmed after the cut', () => {
    expect(normalizeDesignName('é'.repeat(80))).toBe('é'.repeat(MAX_DESIGN_NAME))
    expect([...normalizeDesignName('🚀'.repeat(80))]).toHaveLength(MAX_DESIGN_NAME)
    expect(normalizeDesignName('a'.repeat(59) + '   b')).toBe('a'.repeat(59))
  })
  it('is idempotent', () => {
    const once = normalizeDesignName('  x\u0001y  ' + 'z'.repeat(100))
    expect(normalizeDesignName(once)).toBe(once)
  })
})

describe('display', () => {
  it('falls back to the default name', () => {
    expect(designName({ nodes: [], edges: [], notes: [] })).toBe(DEFAULT_DESIGN_NAME)
    expect(designName({ name: '', nodes: [], edges: [], notes: [] })).toBe('Untitled design')
    expect(designName({ name: 'Shop', nodes: [], edges: [], notes: [] })).toBe('Shop')
  })
  it('puts the name in the browser tab title', () => {
    expect(pageTitle('Shop')).toBe('Shop · System Design Playground')
  })
})

describe('the name in a loaded design', () => {
  it('is kept when valid, cleaned when not, and absent designs load unchanged', () => {
    const ok = sanitizeDiagram(reg, { name: 'Shop', nodes: [], edges: [], notes: [] })!
    expect(ok.diagram.name).toBe('Shop')
    expect(ok.adjusted).toBe(false)
    const dirty = sanitizeDiagram(reg, { name: '  Shop\u0000 ' + 'x'.repeat(100), nodes: [], edges: [], notes: [] })!
    expect(dirty.diagram.name).toBe(('Shop ' + 'x'.repeat(100)).slice(0, 60).trim())
    expect(dirty.adjusted).toBe(true)
    const old = sanitizeDiagram(reg, { nodes: [], edges: [], notes: [] })!
    expect('name' in old.diagram).toBe(false)
    expect(old.adjusted).toBe(false)
    expect('name' in sanitizeDiagram(reg, { name: 42, nodes: [], edges: [], notes: [] })!.diagram).toBe(false)
  })
})
