import type { Registry } from '../api/types'
import { Reg } from '../model/diagram'

// A small registry shared by unit tests so they do not need the Go server.
export const registry: Registry = {
  latency: {
    min: 1,
    max: 200,
    // same first-match table the server serves
    rules: [{ from: 'client', to: 'cdn', ms: 10 }, { from: 'client', ms: 40 }, { from: 'cdn', ms: 40 }, { to: 'externalApi', ms: 20 }, { ms: 5 }],
  },
  speeds: [0.5, 1, 2],
  reasons: [{ id: 'queueFull', label: 'Queue full (503)' }],
  nodes: [
    {
      kind: 'client', name: 'Users', description: '', lesson: '', accent: '#000', width: 160, height: 72,
      canConnectTo: ['appServer'],
      params: [{ key: 'rps', label: 'rps', group: 'g', type: 'number', default: 50, min: 1, max: 5000 }],
    },
    {
      kind: 'appServer', name: 'App Server', description: '', lesson: '', accent: '#000', width: 160, height: 72,
      canConnectTo: ['database'],
      params: [
        { key: 'threads', label: 't', group: 'g', type: 'number', default: 8, min: 1, max: 64 },
        { key: 'order', label: 'o', group: 'g', type: 'select', default: 'fifo', options: [{ value: 'fifo', label: 'Oldest first' }, { value: 'lifo', label: 'Newest first' }] },
        { key: 'dbPool', label: 'p', group: 'g', type: 'number', default: 20, min: 1, max: 200, needsEdgeTo: ['database'] },
        { key: 'cb', label: 'c', group: 'g', type: 'boolean', default: false },
      ],
    },
    { kind: 'database', name: 'Database', description: '', lesson: '', accent: '#000', width: 160, height: 72, canConnectTo: null, params: [] },
  ],
}
export const reg = new Reg(registry)
