import { describe, expect, it } from 'vitest'
import { designUrl, parseHash } from './links'

describe('parseHash', () => {
  it('reads challenge and design links', () => {
    expect(parseHash('#challenge=hello-world')).toEqual({ kind: 'challenge', id: 'hello-world' })
    expect(parseHash('#design=3ibupvsc57zfg')).toEqual({ kind: 'design', id: '3ibupvsc57zfg' })
  })

  it('ignores anything else', () => {
    for (const h of ['', '#', '#tutorial', '#help', '#challenge=', '#design=short', '#design=UPPERCASE12345', '#challenge=../../etc']) {
      expect(parseHash(h)).toBeNull()
    }
  })

  it('tolerates other parameters around the link', () => {
    expect(parseHash('#x=1&challenge=opening-night&y=2')).toEqual({ kind: 'challenge', id: 'opening-night' })
  })

  it('does not match a longer id that merely contains a valid one', () => {
    expect(parseHash('#design=3ibupvsc57zfgxx')).toBeNull()
  })
})

describe('designUrl', () => {
  it('round-trips through parseHash', () => {
    const url = designUrl('https://example.test', '3ibupvsc57zfg')
    expect(url).toBe('https://example.test/#design=3ibupvsc57zfg')
    expect(parseHash(new URL(url).hash)).toEqual({ kind: 'design', id: '3ibupvsc57zfg' })
  })
})
