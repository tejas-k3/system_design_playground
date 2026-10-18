// Performance guard: open a 94-node, ~270-edge design at 5,000 rps in headless Chrome, play it for 10 s and
// measure requestAnimationFrame intervals and long tasks. exit 0 = within the thresholds.
// usage: URL=http://localhost:8091 node stress-frames.mjs
// Headless software rendering is slower than a real GPU, so the thresholds have a margin and one retry.
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const URL_ = process.env.URL ?? 'http://localhost:8091'
const MEDIAN_MS = Number(process.env.MEDIAN_MS ?? 20) // 60 fps is 16.7 ms; allow headless overhead
const P95_MS = Number(process.env.P95_MS ?? 34)
const LONG_TASKS = Number(process.env.LONG_TASKS ?? 0)
const here = dirname(fileURLToPath(import.meta.url))

/** 1 client, 1 balancer, 60 app servers, 20 caches, 12 databases = 94 nodes. */
function bigDesign() {
  const nodes = []
  const edges = []
  const add = (id, kind, x, y, params = {}) => nodes.push({ id, kind, x, y, label: id, params })
  const link = (a, b) => edges.push({ id: `${a}>${b}`, from: a, to: b, latencyMs: 5 })
  add('users', 'client', 0, 400, { rps: 5000, readPct: 90 })
  add('lb', 'loadBalancer', 240, 400)
  for (let i = 0; i < 60; i++) add(`app${i}`, 'appServer', 520, i * 80, { threads: 16 })
  for (let i = 0; i < 20; i++) add(`c${i}`, 'cache', 820, i * 240)
  for (let i = 0; i < 12; i++) add(`db${i}`, 'database', 1120, i * 400)
  link('users', 'lb')
  for (let i = 0; i < 60; i++) {
    link('lb', `app${i}`)
    link(`app${i}`, `c${i % 20}`)
    link(`app${i}`, `db${i % 12}`)
  }
  for (let i = 0; i < 20; i++) link(`c${i}`, `db${i % 12}`)
  return { nodes, edges, notes: [] }
}

async function measure() {
  const d = bigDesign()
  const res = await fetch(`${URL_}/api/designs`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ diagram: d }) })
  if (!res.ok) throw new Error(`could not save the design: ${res.status} ${await res.text()}`)
  const { id } = await res.json()
  const install = `(()=>{window.__f=[];window.__long=0;let last=performance.now();const tick=(t)=>{window.__f.push(t-last);last=t;requestAnimationFrame(tick)};requestAnimationFrame(tick);try{new PerformanceObserver((l)=>{window.__long+=l.getEntries().length}).observe({entryTypes:['longtask']})}catch(e){}return 1})()`
  const read = `(()=>{const f=window.__f.slice(5).sort((a,b)=>a-b);const q=(p)=>f[Math.min(f.length-1,Math.floor(f.length*p))];return JSON.stringify({frames:f.length,median:q(0.5),p95:q(0.95),long:window.__long,comps:+((document.body.innerText.match(/Components\\s+(\\d+)/)||[])[1]||0),requests:+(((document.body.innerText.match(/Requests\\s+([\\d,]+)/)||[])[1]||'0').replace(/,/g,''))})})()`
  const steps = [
    { goto: `${URL_}/#design=${id}`, wait: 3000 },
    { click: 'Play', wait: 1500 },
    { eval: install, wait: 10000 },
    { eval: read, log: 'RESULT' },
  ]
  const out = execFileSync('node', [join(here, 'walk.mjs'), '1440', '900', '/tmp/review/stress', JSON.stringify(steps)], { encoding: 'utf8' })
  const line = out.split('\n').find((l) => l.startsWith('RESULT'))
  if (!line) throw new Error('no result from the browser:\n' + out)
  return JSON.parse(JSON.parse(line.slice(line.indexOf(' ') + 1)))
}

let r = await measure()
const bad = (x) => x.median > MEDIAN_MS || x.p95 > P95_MS || x.long > LONG_TASKS
const notRunning = (x) => x.comps !== 94 || x.requests < 1000
if (bad(r)) {
  console.log('first attempt over the limits, retrying once:', JSON.stringify(r))
  r = await measure()
}
if (notRunning(r)) {
  console.log(`FAIL: the page does not show a running 94-node design (components ${r.comps}, requests ${r.requests}); the measurement would mean nothing`)
  process.exit(1)
}
console.log(`94 nodes at 5,000 rps: ${r.frames} frames, median ${r.median.toFixed(1)} ms, p95 ${r.p95.toFixed(1)} ms, long tasks ${r.long}`)
if (bad(r)) {
  console.log(`FAIL: limits are median <= ${MEDIAN_MS} ms, p95 <= ${P95_MS} ms, long tasks <= ${LONG_TASKS}`)
  process.exit(1)
}
console.log('PASS')
