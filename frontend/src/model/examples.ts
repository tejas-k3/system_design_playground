import type { Diagram } from '../api/types'

/** A starter design: users → load balancer → two app servers → cache → database. */
export function exampleDiagram(): Diagram {
  const n = (id: string, kind: string, x: number, y: number, label: string, params = {}) => ({ id, kind, x, y, label, params })
  const e = (from: string, to: string, latencyMs: number) => ({ id: `${from}>${to}`, from, to, latencyMs })
  return {
    nodes: [
      n('users', 'client', 0, 150, 'Users', { rps: 150, readPct: 85 }),
      n('lb', 'loadBalancer', 240, 150, 'Load Balancer'),
      n('app1', 'appServer', 480, 60, 'App Server'),
      n('app2', 'appServer', 480, 240, 'App Server 2'),
      n('cache', 'cache', 720, 60, 'Cache'),
      n('db', 'database', 960, 150, 'Database'),
    ],
    edges: [
      e('users', 'lb', 40),
      e('lb', 'app1', 5),
      e('lb', 'app2', 5),
      e('app1', 'cache', 5),
      e('app2', 'cache', 5),
      e('cache', 'db', 5),
      e('app1', 'db', 5),
      e('app2', 'db', 5),
    ],
    notes: [{ id: 'note1', x: 480, y: 360, text: 'Press Play, then change the cache size\nor kill a server mid-run.' }],
  }
}
