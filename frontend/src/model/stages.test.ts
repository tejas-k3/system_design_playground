import { describe, expect, it } from 'vitest'
import { countdown, markText, stageAfterKey, stageInfo } from './stages'
import { STAGES } from './timer'

describe('stage prompts', () => {
  it('has a prompt and a tool for every stage', () => {
    for (const s of STAGES) {
      const i = stageInfo(s.id)
      expect(i.prompt.length).toBeGreaterThan(40)
      expect(i.tool.length).toBeGreaterThan(3)
    }
  })
  it('asks for three functional and three non-functional requirements first', () => {
    expect(stageInfo('requirements').prompt).toMatch(/three functional/i)
    expect(stageInfo('requirements').prompt).toMatch(/non-functional/i)
  })
})

describe('countdown', () => {
  it('shows minutes and seconds, rounding the remaining time up so it never reads 0:00 while time is left', () => {
    expect(countdown(45 * 60_000)).toBe('45:00')
    expect(countdown(9 * 60_000 + 5_000)).toBe('9:05')
    expect(countdown(59_001)).toBe('1:00')
    expect(countdown(1)).toBe('0:01')
    expect(countdown(0)).toBe('0:00')
    expect(countdown(-5)).toBe('0:00')
  })
  it('shows hours for long sessions', () => {
    expect(countdown(75 * 60_000)).toBe('1:15:00')
  })
})

describe('stageAfterKey', () => {
  it('moves along the stages with the arrow keys and stops at the ends', () => {
    expect(stageAfterKey('api', 'ArrowRight')).toBe('data')
    expect(stageAfterKey('api', 'ArrowLeft')).toBe('estimation')
    expect(stageAfterKey('requirements', 'ArrowLeft')).toBe('requirements')
    expect(stageAfterKey('tradeoffs', 'ArrowRight')).toBe('tradeoffs')
  })
  it('jumps with Home and End, and ignores other keys', () => {
    expect(stageAfterKey('api', 'Home')).toBe('requirements')
    expect(stageAfterKey('api', 'End')).toBe('tradeoffs')
    expect(stageAfterKey('api', 'a')).toBeNull()
  })
})

describe('markText', () => {
  it('says each threshold in words for the live region', () => {
    expect(markText('half')).toBe('Half the time has passed.')
    expect(markText('tenLeft')).toBe('Ten minutes left.')
    expect(markText('fiveLeft')).toBe('Five minutes left.')
    expect(markText('oneLeft')).toBe('One minute left.')
    expect(markText('over')).toBe('Time is up.')
  })
})
