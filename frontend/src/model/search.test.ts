import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { centerOn, searchNodes } from './search'

const n = (id: string, kind: string, label: string) => ({ id, kind, x: 0, y: 0, label, params: {} })
const d: Diagram = {
  nodes: [n('1', 'appServer', 'Checkout API'), n('2', 'database', 'Orders DB'), n('3', 'cache', 'Session cache'), n('4', 'appServer', 'Search API'), n('5', 'database', 'Cart')],
  edges: [],
  notes: [],
}
const kindName = (k: string) => ({ appServer: 'App Server', database: 'Database', cache: 'Cache' })[k] ?? k
const ids = (q: string) => searchNodes(d, kindName, q).map((x) => x.id)

describe('searchNodes', () => {
  it('an empty query lists everything in diagram order', () => {
    expect(ids('')).toEqual(['1', '2', '3', '4', '5'])
    expect(ids('   ')).toEqual(['1', '2', '3', '4', '5'])
  })
  it('matches the label, ignoring case', () => {
    expect(ids('api')).toEqual(['1', '4'])
    expect(ids('ORDERS')).toEqual(['2'])
  })
  it('matches the component type too', () => {
    expect(ids('database')).toEqual(['2', '5'])
  })
  it('label that starts with the query comes before one that merely contains it', () => {
    // Session cache and Search API start with s; Orders DB contains it; the rest only have it in their type name
    expect(ids('s')).toEqual(['3', '4', '2', '1', '5'])
  })
  it('matches every word of a multi-word query, in any order', () => {
    expect(ids('api search')).toEqual(['4'])
    expect(ids('db orders')).toEqual(['2'])
  })
  it('nothing matches gives an empty list', () => {
    expect(ids('zzz')).toEqual([])
  })
})

describe('centerOn', () => {
  it('puts the middle of the rectangle in the middle of the canvas at the same zoom', () => {
    const v = centerOn({ x: 100, y: 200, w: 160, h: 72 }, { w: 1000, h: 600 }, { scrollX: 5, scrollY: 5, zoom: 1 })
    expect(v.zoom).toBe(1)
    expect((100 + 80) + v.scrollX).toBeCloseTo(500)
    expect((200 + 36) + v.scrollY).toBeCloseTo(300)
  })
  it('zooms in when the view is very small so the result is readable', () => {
    expect(centerOn({ x: 0, y: 0, w: 160, h: 72 }, { w: 1000, h: 600 }, { scrollX: 0, scrollY: 0, zoom: 0.2 }).zoom).toBeGreaterThanOrEqual(0.6)
  })
})
