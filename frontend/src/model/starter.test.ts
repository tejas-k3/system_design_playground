import { describe, expect, it } from 'vitest'
import { reg } from '../test/fixtures'
import { sanitizeDiagram } from './diagram'
import { starterDiagram } from './starter'

describe('starterDiagram', () => {
  it('is users, a balancer, N app servers and a database, wired in a line', () => {
    const d = starterDiagram({ peakQps: 1200, readPct: 90, servers: 3 })
    expect(d.nodes.map((n) => n.kind)).toEqual(['client', 'loadBalancer', 'appServer', 'appServer', 'appServer', 'database'])
    expect(d.edges.map((e) => e.id)).toEqual(['users>lb', 'lb>app1', 'lb>app2', 'lb>app3', 'app1>db', 'app2>db', 'app3>db'])
  })
  it('puts the learner’s peak load and read share on the users', () => {
    const d = starterDiagram({ peakQps: 1200, readPct: 90, servers: 2 })
    expect(d.nodes[0].params).toEqual({ rps: 1200, readPct: 90 })
  })
  it('survives the sanitizer unchanged, so it is a valid design', () => {
    const d = starterDiagram({ peakQps: 1200, readPct: 90, servers: 4 })
    const clean = sanitizeDiagram(reg, d)
    expect(clean?.adjusted).toBe(false)
    expect(clean?.diagram.nodes).toHaveLength(d.nodes.length)
    expect(clean?.diagram.edges).toHaveLength(d.edges.length)
  })
  it('keeps the request rate inside what a client can send and the fleet inside what fits on screen', () => {
    const d = starterDiagram({ peakQps: 90_000, readPct: 80, servers: 400 })
    expect(d.nodes[0].params.rps).toBe(5000)
    expect(d.nodes.filter((n) => n.kind === 'appServer')).toHaveLength(12)
  })
  it('never builds fewer than one server and rounds nonsense to something sensible', () => {
    expect(starterDiagram({ peakQps: NaN, readPct: 200, servers: NaN }).nodes.filter((n) => n.kind === 'appServer')).toHaveLength(1)
    expect(starterDiagram({ peakQps: NaN, readPct: 200, servers: NaN }).nodes[0].params).toEqual({ rps: 50, readPct: 100 })
  })
})
