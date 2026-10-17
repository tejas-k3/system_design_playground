import { describe, expect, it } from 'vitest'
import { groupKinds } from './library'

describe('groupKinds', () => {
  it('groups components by what they are for, in a stable order, keeping the server order inside a group', () => {
    const groups = groupKinds(['database', 'client', 'cache', 'appServer', 'loadBalancer', 'queue'])
    expect(groups).toEqual([
      { title: 'Traffic', kinds: ['client', 'loadBalancer'] },
      { title: 'Compute', kinds: ['appServer'] },
      { title: 'Data', kinds: ['database', 'cache'] },
      { title: 'Async and external', kinds: ['queue'] },
    ])
  })
  it('never loses a kind it does not know: it goes in Other', () => {
    expect(groupKinds(['client', 'quantumDb'])).toEqual([
      { title: 'Traffic', kinds: ['client'] },
      { title: 'Other', kinds: ['quantumDb'] },
    ])
  })
  it('omits empty groups', () => {
    expect(groupKinds([])).toEqual([])
  })
})
