import { describe, expect, it } from 'vitest'
import { plain, splitMarkup, GLOSSARY } from './glossary'

const id = GLOSSARY[0].id

describe('markup in texts that are not prose', () => {
  it('plain() gives the shown words for a prediction option, with no braces left', () => {
    expect(plain(`Add a {{${id}|fast layer}} in front`)).toBe('Add a fast layer in front')
    expect(plain(`Use {{${id}}}`)).not.toContain('{{')
  })
  it('splits question, explanation and a why text into words and terms, and an unknown id stays plain words', () => {
    const pieces = splitMarkup(`Because of {{${id}|this}} and {{no-such-term|that}}.`)
    expect(pieces.filter((p) => typeof p !== 'string')).toHaveLength(1)
    expect(plain(`Because of {{${id}|this}} and {{no-such-term|that}}.`)).toBe('Because of this and that.')
  })
  it('leaves text without markup unchanged', () => {
    expect(plain('No markup, 50% of calls.')).toBe('No markup, 50% of calls.')
    expect(splitMarkup('No markup')).toEqual(['No markup'])
  })
})
