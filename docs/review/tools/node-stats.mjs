// usage: node node-stats.mjs <scenarioId> <simSeconds>   (server on :8091) prints live per-node and per-edge stats at that sim time
const sid = process.argv[2], secs = +process.argv[3] || 8
const sc = await (await fetch(`http://localhost:8091/api/scenarios/${sid}`)).json()
const ws = new WebSocket('ws://localhost:8091/api/sim')
let last = {}, n = 0
ws.onopen = () => { ws.send(JSON.stringify({ type: 'init', diagram: sc.diagram, scenarioId: sid, seed: 1, speed: 1 })); ws.send(JSON.stringify({ type: 'play' })) }
ws.onmessage = (e) => {
  const m = JSON.parse(e.data)
  if (m.type !== 'frame' || !m.stats) return
  const t = m.stats.global.now
  if (t > secs && !last.done) {
    last.done = true
    for (const [id, s] of Object.entries(m.stats.nodes)) console.log(id, 'arr', s.arr.toFixed(1), 'ok', s.ok.toFixed(1), 'err', s.err.toFixed(1), 'util', s.util.toFixed(2), 'busy', s.busy, '/', s.cap, 'q', s.qLen)
    for (const [id, s] of Object.entries(m.stats.edges)) console.log('edge', id, 'rps', s.rps.toFixed(1))
    console.log('t', t, 'run', JSON.stringify({ ok: m.stats.global.run.ok, resolved: m.stats.global.run.resolved, pending: m.stats.global.run.pending, p50: m.stats.global.run.p50 }))
    ws.close(); process.exit(0)
  }
}
setTimeout(() => process.exit(1), 60000)
