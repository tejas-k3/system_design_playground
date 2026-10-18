import type { Diagram, Edge, Node } from '../api/types'

const MAX_SERVERS = 12 // more would not fit on screen; the numbers still say how many are really needed
const MAX_RPS = 5000 // the most one Users component can send

/** A first design from the learner's numbers: users, a balancer, N app servers and a database. */
export function starterDiagram(x: { peakQps: number; readPct: number; servers: number }): Diagram {
  const rps = Number.isFinite(x.peakQps) && x.peakQps > 0 ? Math.min(MAX_RPS, Math.max(1, Math.round(x.peakQps))) : 50
  const readPct = Number.isFinite(x.readPct) ? Math.min(100, Math.max(0, Math.round(x.readPct))) : 80
  const n = Number.isFinite(x.servers) ? Math.min(MAX_SERVERS, Math.max(1, Math.round(x.servers))) : 1
  const node = (id: string, kind: string, px: number, py: number, label: string, params: Node['params'] = {}): Node => ({ id, kind, x: px, y: py, label, params })
  const edge = (from: string, to: string, latencyMs: number): Edge => ({ id: `${from}>${to}`, from, to, latencyMs })
  const gap = 90
  const mid = ((n - 1) * gap) / 2
  const nodes: Node[] = [node('users', 'client', 0, mid, 'Users', { rps, readPct }), node('lb', 'loadBalancer', 240, mid, 'Load Balancer')]
  const edges: Edge[] = [edge('users', 'lb', 40)]
  for (let i = 1; i <= n; i++) {
    nodes.push(node(`app${i}`, 'appServer', 480, (i - 1) * gap, i === 1 ? 'App Server' : `App Server ${i}`))
    edges.push(edge('lb', `app${i}`, 5))
  }
  nodes.push(node('db', 'database', 720, mid, 'Database'))
  for (let i = 1; i <= n; i++) edges.push(edge(`app${i}`, 'db', 5))
  return { nodes, edges, notes: [] }
}
